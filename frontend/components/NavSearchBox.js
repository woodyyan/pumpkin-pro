'use client'

import { useState, useRef, useEffect, useCallback } from 'react'
import { useRouter } from 'next/router'

const DEBOUNCE_MS = 300
const MIN_QUERY_LEN = 2
const MAX_RESULTS = 8

export default function NavSearchBox() {
  const router = useRouter()
  const [query, setQuery] = useState('')
  const [results, setResults] = useState([])
  const [isOpen, setIsOpen] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const wrapperRef = useRef(null)
  const debounceRef = useRef(null)

  // Close dropdown on outside click
  useEffect(() => {
    function handleClickOutside(e) {
      if (wrapperRef.current && !wrapperRef.current.contains(e.target)) {
        setIsOpen(false)
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  // Keyboard navigation
  const [activeIdx, setActiveIdx] = useState(-1)

  const doSearch = useCallback(async (q) => {
    if (q.length < MIN_QUERY_LEN) {
      setResults([])
      setIsOpen(false)
      return
    }
    setIsLoading(true)
    try {
      const res = await fetch(`/api/search?q=${encodeURIComponent(q)}&limit=${MAX_RESULTS}`)
      const data = await res.json()
      setResults(data.results || [])
      setIsOpen(true)
    } catch {
      setResults([])
    } finally {
      setIsLoading(false)
    }
  }, [])

  // Debounced search
  const handleInputChange = (e) => {
    const v = e.target.value
    setQuery(v)
    setActiveIdx(-1)
    if (debounceRef.current) clearTimeout(debounceRef.current)
    debounceRef.current = setTimeout(() => doSearch(v), DEBOUNCE_MS)
  }

  const clearInput = () => {
    setQuery('')
    setResults([])
    setIsOpen(false)
    setActiveIdx(-1)
    if (debounceRef.current) clearTimeout(debounceRef.current)
  }

  const handleSelect = (item) => {
    setIsOpen(false)
    setQuery('')
    setResults([])
    // Convert raw code to symbol with exchange suffix
    let symbol
    if (item.exchange === 'HKEX') {
      symbol = `${String(item.code).padStart(5, '0')}.HK`
    } else {
      const c = String(item.code).padStart(6, '0')
      symbol = (c.startsWith('6') || c.startsWith('9')) ? `${c}.SH` : `${c}.SZ`
    }
    window.open(`/live-trading/${symbol}`, '_blank')
  }

  const handleKeyDown = (e) => {
    if (!isOpen || results.length === 0) return
    switch (e.key) {
      case 'ArrowDown':
        e.preventDefault()
        setActiveIdx((i) => Math.min(i + 1, results.length - 1))
        break
      case 'ArrowUp':
        e.preventDefault()
        setActiveIdx((i) => Math.max(i - 1, -1))
        break
      case 'Enter':
        e.preventDefault()
        if (activeIdx >= 0 && activeIdx < results.length) {
          handleSelect(results[activeIdx])
        }
        break
      case 'Escape':
        setIsOpen(false)
        setActiveIdx(-1)
        break
    }
  }

  return (
    <div ref={wrapperRef} className="relative w-[200px] focus-within:w-[280px] transition-all duration-200">
      {/* Input */}
      <div className="flex items-center bg-white/10 border border-white/15 rounded-lg px-3 py-1.5 focus-within:border-primary/40 focus-within:bg-white/12 transition">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="shrink-0 text-white/40 mr-1.5">
          <circle cx="11" cy="11" r="8" /><path d="M21 21l-4.35-4.35" />
        </svg>
        <input
          type="text"
          value={query}
          onChange={handleInputChange}
          onKeyDown={handleKeyDown}
          onFocus={() => { if (results.length > 0 && query.length >= MIN_QUERY_LEN) setIsOpen(true) }}
          placeholder="搜索代码或名称"
          className="bg-transparent text-sm text-white placeholder-white/30 outline-none min-w-0 flex-1"
        />
        {query && (
          <button type="button" onClick={clearInput} className="shrink-0 ml-1 text-white/40 hover:text-white/70 transition" aria-label="清除">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M18 6L6 18M6 6l12 12" /></svg>
          </button>
        )}
        {isLoading && (
          <span className="shrink-0 ml-1 w-3 h-3 border border-white/20 border-t-white/60 rounded-full animate-spin" />
        )}
      </div>

      {/* Dropdown */}
      {isOpen && (results.length > 0 || (query.length >= MIN_QUERY_LEN && !isLoading)) && (
        <div className="absolute top-full left-0 right-0 mt-1.5 bg-slate-950 border border-white/10 rounded-xl shadow-2xl z-50 overflow-hidden">
          {results.length > 0 ? (
            <ul>
              {results.map((item, i) => (
                <li key={item.code}>
                  <button
                    type="button"
                    onClick={() => handleSelect(item)}
                    onMouseEnter={() => setActiveIdx(i)}
                    className={`w-full flex items-center justify-between px-4 py-2.5 text-left text-sm transition ${
                      i === activeIdx ? 'bg-primary/15 text-primary' : 'text-white/80 hover:bg-white/5'
                    }`}
                  >
                    <span>
                      <span className={`font-mono font-semibold ${i === activeIdx ? '' : 'text-primary/80'}`}>{item.code}</span>
                      <span className="ml-2 text-white/50">{item.name}</span>
                      {item.exchange === 'HKEX' && (
                        <span className="ml-1.5 inline-flex items-center px-1 rounded text-[10px] font-medium bg-blue-500/20 text-blue-300">HK</span>
                      )}
                    </span>
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="shrink-0 opacity-30">
                      <path d="M7 17L17 7M7 7h10v10" />
                    </svg>
                  </button>
                </li>
              ))}
            </ul>
          ) : (
            <div className="px-4 py-3 text-center text-sm text-white/30">未找到匹配股票</div>
          )}
        </div>
      )}
    </div>
  )
}
