import { useEffect, useMemo, useRef, useState } from 'react'

import {
  QUADRANT_SEARCH_MIN_QUERY_LEN,
  buildQuadrantDetailSymbol,
  normalizeQuadrantMarket,
  normalizeQuadrantStockCode,
} from '../lib/quadrant-search'

export default function QuadrantSearchBox({
  market = 'ASHARE',
  query = '',
  results = [],
  selectedCode = '',
  disabled = false,
  onQueryChange,
  onSelect,
  onClear,
}) {
  const normalizedMarket = normalizeQuadrantMarket(market)
  const wrapperRef = useRef(null)
  const [isOpen, setIsOpen] = useState(false)
  const [activeIdx, setActiveIdx] = useState(-1)
  const normalizedSelectedCode = normalizeQuadrantStockCode(selectedCode, normalizedMarket)
  const showEmptyState = !disabled && query.trim().length >= QUADRANT_SEARCH_MIN_QUERY_LEN && results.length === 0
  const helperText = useMemo(() => (
    normalizedMarket === 'HKEX'
      ? '仅搜索当前港股四象限中的股票，支持代码或名称。'
      : '仅搜索当前 A 股四象限中的股票，支持代码或名称。'
  ), [normalizedMarket])

  useEffect(() => {
    function handleClickOutside(event) {
      if (wrapperRef.current && !wrapperRef.current.contains(event.target)) {
        setIsOpen(false)
        setActiveIdx(-1)
      }
    }

    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [])

  useEffect(() => {
    if (disabled) {
      setIsOpen(false)
      setActiveIdx(-1)
      return
    }
    if (results.length > 0 || showEmptyState) {
      setIsOpen(true)
      return
    }
    setIsOpen(false)
    setActiveIdx(-1)
  }, [disabled, results.length, showEmptyState])

  const handleInputChange = (event) => {
    onQueryChange?.(event.target.value)
    setActiveIdx(-1)
  }

  const handleSelect = (item) => {
    onSelect?.(item)
    setIsOpen(false)
    setActiveIdx(-1)
  }

  const handleClear = () => {
    onClear?.()
    setIsOpen(false)
    setActiveIdx(-1)
  }

  const handleKeyDown = (event) => {
    if (!isOpen || results.length === 0) return
    switch (event.key) {
      case 'ArrowDown':
        event.preventDefault()
        setActiveIdx((idx) => Math.min(idx + 1, results.length - 1))
        break
      case 'ArrowUp':
        event.preventDefault()
        setActiveIdx((idx) => Math.max(idx - 1, -1))
        break
      case 'Enter':
        event.preventDefault()
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
    <div ref={wrapperRef} className="relative">
      <div className="flex flex-col gap-2 md:flex-row md:items-center md:justify-between">
        <div className="min-w-0">
          <div className="text-xs font-medium text-white/65">在当前{normalizedMarket === 'HKEX' ? '港股' : 'A 股'}四象限中搜索</div>
          <div className="mt-1 text-[11px] text-white/35">{helperText}</div>
        </div>
        <div className="inline-flex shrink-0 items-center gap-1 rounded-full border border-primary/15 bg-primary/5 px-2.5 py-1 text-[11px] text-primary/80">
          <span className="inline-block h-1.5 w-1.5 rounded-full bg-primary/80" />
          {normalizedMarket === 'HKEX' ? '港股独立搜索' : 'A 股独立搜索'}
        </div>
      </div>

      <div className={`mt-3 flex items-center rounded-xl border px-3 py-2 transition ${disabled ? 'border-border/40 bg-black/10 text-white/30' : 'border-border bg-black/20 text-white focus-within:border-primary/45'}`}>
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" className="mr-2 shrink-0 text-white/35">
          <circle cx="11" cy="11" r="8" />
          <path d="M21 21l-4.35-4.35" />
        </svg>
        <input
          type="text"
          value={query}
          disabled={disabled}
          onChange={handleInputChange}
          onKeyDown={handleKeyDown}
          onFocus={() => {
            if (!disabled && (results.length > 0 || showEmptyState)) setIsOpen(true)
          }}
          placeholder={disabled ? '四象限数据加载完成后即可搜索' : '输入股票代码或名称，例如 600519 / 腾讯'}
          className="min-w-0 flex-1 bg-transparent text-sm text-white placeholder-white/30 outline-none disabled:cursor-not-allowed"
        />
        {(query || normalizedSelectedCode) && !disabled && (
          <button
            type="button"
            onClick={handleClear}
            className="ml-2 shrink-0 text-white/35 transition hover:text-white/70"
            aria-label="清除四象限搜索"
          >
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M18 6L6 18M6 6l12 12" />
            </svg>
          </button>
        )}
      </div>

      {isOpen && !disabled && (results.length > 0 || showEmptyState) && (
        <div className="absolute left-0 right-0 top-full z-20 mt-2 overflow-hidden rounded-2xl border border-white/10 bg-slate-950/95 shadow-2xl backdrop-blur">
          {results.length > 0 ? (
            <ul>
              {results.map((item, index) => {
                const normalizedItemCode = normalizeQuadrantStockCode(item.c, normalizedMarket)
                const isActive = index === activeIdx
                const isSelected = normalizedItemCode === normalizedSelectedCode
                return (
                  <li key={normalizedItemCode}>
                    <button
                      type="button"
                      onClick={() => handleSelect(item)}
                      onMouseEnter={() => setActiveIdx(index)}
                      className={`w-full border-b border-white/5 px-4 py-3 text-left transition last:border-b-0 ${isActive ? 'bg-primary/15' : isSelected ? 'bg-white/[0.04]' : 'hover:bg-white/[0.04]'}`}
                    >
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2 text-sm text-white">
                            <span className={`font-medium ${isActive || isSelected ? 'text-primary' : ''}`}>{item.n}</span>
                            <span className="font-mono text-[12px] text-white/45">{normalizedItemCode}</span>
                          </div>
                          <div className="mt-1 flex flex-wrap items-center gap-2 text-[11px] text-white/45">
                            <span className={`rounded-full px-2 py-0.5 ${quadrantBadgeClass(item.q)}`}>{item.q}区</span>
                            <span>机会 {Number(item.o || 0).toFixed(1)}</span>
                            <span>风险 {Number(item.r || 0).toFixed(1)}</span>
                          </div>
                        </div>
                        <span className="shrink-0 text-[11px] text-white/25">{buildQuadrantDetailSymbol(item.c, normalizedMarket)}</span>
                      </div>
                    </button>
                  </li>
                )
              })}
            </ul>
          ) : (
            <div className="px-4 py-4 text-center text-sm text-white/35">当前市场未找到匹配股票</div>
          )}
        </div>
      )}
    </div>
  )
}

function quadrantBadgeClass(quadrant) {
  switch (quadrant) {
    case '机会':
      return 'bg-emerald-500/12 text-emerald-300'
    case '拥挤':
      return 'bg-amber-500/12 text-amber-300'
    case '泡沫':
      return 'bg-rose-500/12 text-rose-300'
    case '防御':
      return 'bg-white/8 text-white/70'
    default:
      return 'bg-blue-500/12 text-blue-300'
  }
}
