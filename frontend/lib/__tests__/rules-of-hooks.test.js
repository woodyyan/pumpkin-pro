// ── Static analysis: React Rules of Hooks enforcement ──
//
// Catches the production crash pattern: "Rendered more hooks than during
// the previous render" caused by calling useState/useEffect/useRef AFTER
// a conditional early return (e.g., `if (!ready) return ...`).
//
// This test scans all .js/.jsx files under frontend/ and verifies that
// within each function component, ALL hook calls appear BEFORE any
// conditional/early return statements.
//
// Rule: In a function component body, once you hit a `return` statement
// (that is not inside a nested function/closure), no more hooks may follow.

import { describe, it } from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync, readdirSync, statSync } from 'node:fs'
import { join, extname } from 'node:path'

const FRONTEND_DIR = join(import.meta.dirname, '..', '..')

// ── Hook detection patterns ──
const HOOK_PATTERN = /^(use|useState|useEffect|useRef|useMemo|useCallback|useContext|useReducer|useLayoutEffect|useImperativeHandle|useDebugValue|useId|useSyncExternalStore|useTransition|useDeferredValue)\b/

/**
 * Check if a line contains a top-level hook call.
 * Ignores hooks inside nested functions (callbacks), object literals, or JSX.
 * This is a simplified heuristic — good enough to catch the AccountEntry bug pattern.
 */
function isHookCall(line) {
  // Must match known hook names at the start of an expression
  const trimmed = line.trim()
  // Match: const x = useXxx(  or  useXxx(  or  } = useXxx(
  if (/^(?:const|let|var)\s+\{?[\w]*\}?\s*=\s*(use\w+)\(/.test(trimmed)) return true
  if (/^(use\w+)\(/.test(trimmed)) return true
  // Destructured: const [a, b] = useState(...)
  if (/^(?:const|let|var)\s+\[[^\]]*\]\s*=\s*(use\w+)\(/.test(trimmed)) return true
  return false
}

/**
 * Check if a line is an early/conditional return statement at the top level of the component.
 */
function isEarlyReturn(line, depth) {
  const trimmed = line.trim()
  // Only consider returns at component body level (depth === 0 or 1 depending on formatting)
  if (!trimmed.startsWith('return ')) return false
  // Ignore returns inside callbacks/nested functions (they have deeper indentation)
  // A simple heuristic: early returns in components usually have low indentation
  if (depth > 2) return false
  return true
}

/**
 * Analyze a single function component's source code for Rules of Hooks violations.
 *
 * Returns { ok, errors[] }
 *
 * Strategy:
 *   - Track brace depth to distinguish component-level code from nested closures
 *   - Scan line-by-line: record positions of hook calls and return statements
 *   - If any hook call appears AFTER a component-level return → violation
 */
function analyzeComponent(sourceCode, functionName, filePath) {
  const lines = sourceCode.split('\n')
  const errors = []

  let foundReturnLine = -1    // Line number of first early return
  let braceDepth = 0           // Current brace nesting depth
  const hookLines = []         // { lineNum, hookName }

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i]
    const lineNum = i + 1

    // Track brace depth (simplified — doesn't handle strings/comments perfectly,
    // but sufficient for well-formatted React code)
    for (const ch of line) {
      if (ch === '{') braceDepth++
      if (ch === '}') braceDepth--
    }

    // Only check at component body level (depth 1, since we're inside the function)
    const effectiveDepth = braceDepth

    if (isHookCall(line)) {
      hookLines.push({ lineNum, raw: line.trim() })

      // If we already saw an early return at shallow depth → VIOLATION
      if (foundReturnLine > 0 && effectiveDepth <= 1) {
        errors.push(
          `Hook "${line.trim()}" at line ${lineNum} appears AFTER early return at line ${foundReturnLine}. ` +
          `This violates Rules of Hooks and causes "Rendered more hooks" runtime error.`
        )
      }
    }

    // Detect early return at component level (after hooks should have been declared)
    if (isEarlyReturn(line, effectiveDepth) && effectiveDepth <= 1) {
      // Mark the FIRST early return — any hook after this is suspect
      if (foundReturnLine === -1) {
        foundReturnLine = lineNum
      }
    }
  }

  // Also flag the anti-pattern: early return exists AND there are hooks after it
  // (even if our simple parser missed them due to formatting)
  if (foundReturnLine > 0 && hookLines.some((h) => h.lineNum > foundReturnLine)) {
    // Already captured above
  }

  return { ok: errors.length === 0, errors, foundReturnLine, hookCount: hookLines.length }
}

/**
 * Recursively find all .js/.jsx files in a directory.
 */
function findJsFiles(dir, maxDepth = 4, _currentDepth = 0) {
  let results = []
  try {
    const entries = readdirSync(dir)
    for (const entry of entries) {
      if (entry.startsWith('.') || entry === 'node_modules' || entry === '.next') continue
      const fullPath = join(dir, entry)
      const stat = statSync(fullPath)
      if (stat.isDirectory()) {
        if (_currentDepth < maxDepth) {
          results = results.concat(findJsFiles(fullPath, maxDepth, _currentDepth + 1))
        }
      } else if (extname(entry) === '.js' || extname(entry) === '.jsx') {
        results.push(fullPath)
      }
    }
  } catch {
    // Skip unreadable directories
  }
  return results
}

// ── Known safe patterns to skip (false positive suppression) ──
// These are files/components where early-returns before hooks are intentional
// and safe (e.g., they never change their return path between renders).
const SKIP_FILES = new Set([
  // Add file basenames here if needed
])

describe('React Rules of Hooks: static analysis', () => {

  it('no hooks are called after conditional early returns in any component', () => {
    const jsFiles = findJsFiles(FRONTEND_DIR).filter(
      (f) => !f.includes('__tests__') && !f.includes('node_modules') && !SKIP_FILES.has(f.split('/').pop())
    )

    const failures = []

    for (const filePath of jsFiles) {
      try {
        const source = readFileSync(filePath, 'utf-8')

        // Find all function components (functions starting with uppercase or assigned to const with uppercase)
        // Simple regex-based extraction
        const fnComponentRegex =
          /(?:^|\n)\s*(?:function\s+([A-Z]\w*)|(?:const|let|var)\s+([A-Z]\w*)\s*=\s*(?:async\s+)?(?:\([^)]*\)|[^=])?\s*=>)/g

        let match
        while ((match = fnComponentRegex.exec(source)) !== null) {
          const funcName = match[1] || match[2]
          const funcStartIdx = match.index

          // Extract function body (roughly balanced braces from the opening {)
          const openBrace = source.indexOf('{', funcStartIdx)
          if (openBrace === -1) continue

          let depth = 0
          let endIdx = openBrace
          for (let j = openBrace; j < source.length; j++) {
            if (source[j] === '{') depth++
            if (source[j] === '}') {
              depth--
              if (depth === 0) {
                endIdx = j + 1
                break
              }
            }
          }

          const funcBody = source.substring(openBrace + 1, endIdx)
          const result = analyzeComponent(funcBody, funcName, filePath)

          if (!result.ok) {
            for (const err of result.errors) {
              failures.push(`  📄 ${filePath}\n     Component: ${funcName}()\n     ${err}`)
            }
          }
        }
      } catch {
        // Skip files that can't be parsed
      }
    }

    assert.equal(
      failures.length, 0,
      `Found ${failures.length} React Rules of Hooks violation(s):\n\n${failures.join('\n\n')}`
    )
  })
})
