import React from 'react'

function placementClasses(placement) {
  switch (placement) {
    case 'bottom-left':
      return {
        panel: 'left-0 top-full mt-2',
        arrow: 'left-3 top-0 -translate-y-full border-b-[#101418]/95',
      }
    case 'bottom-right':
      return {
        panel: 'right-0 top-full mt-2',
        arrow: 'right-3 top-0 -translate-y-full border-b-[#101418]/95',
      }
    case 'top-left':
      return {
        panel: 'left-0 bottom-full mb-2',
        arrow: 'left-3 top-full border-t-[#101418]/95',
      }
    case 'top-right':
      return {
        panel: 'right-0 bottom-full mb-2',
        arrow: 'right-3 top-full border-t-[#101418]/95',
      }
    case 'bottom':
      return {
        panel: 'left-1/2 top-full mt-2 -translate-x-1/2',
        arrow: 'left-1/2 top-0 -translate-x-1/2 -translate-y-full border-b-[#101418]/95',
      }
    case 'top':
    default:
      return {
        panel: 'left-1/2 bottom-full mb-2 -translate-x-1/2',
        arrow: 'left-1/2 top-full -translate-x-1/2 border-t-[#101418]/95',
      }
  }
}

export function InfoTip({
  text,
  className = '',
  iconClassName = '',
  panelClassName = '',
  placement = 'top',
  widthClassName = 'w-56 sm:w-64',
  ariaLabel = '查看字段说明',
}) {
  if (!text) return null

  const classes = placementClasses(placement)

  return (
    <span className={`group/info-tip relative inline-flex align-middle ${className}`}>
      <button
        type="button"
        className={`inline-flex h-4 w-4 items-center justify-center rounded-full border border-white/15 text-[10px] font-semibold text-white/40 transition hover:border-primary/40 hover:text-primary focus:outline-none focus:ring-2 focus:ring-primary/25 ${iconClassName}`}
        aria-label={ariaLabel}
        onClick={(event) => event.stopPropagation()}
        onMouseDown={(event) => event.stopPropagation()}
        onKeyDown={(event) => event.stopPropagation()}
      >
        i
      </button>
      <span className={`pointer-events-none absolute z-30 hidden rounded-lg border border-white/10 bg-[#101418]/95 px-3 py-2 text-[11px] font-normal leading-5 text-white/80 shadow-2xl backdrop-blur-sm group-hover/info-tip:block group-focus-within/info-tip:block ${widthClassName} ${classes.panel} ${panelClassName}`}>
        {text}
        <span className={`absolute h-0 w-0 border-x-4 border-x-transparent border-y-4 border-y-transparent ${classes.arrow}`} />
      </span>
    </span>
  )
}

export function LabelWithInfo({
  label,
  tooltip,
  className = '',
  labelClassName = '',
  tipClassName = '',
  tipPlacement = 'top',
  tipWidthClassName,
  tipPanelClassName,
}) {
  return (
    <span className={`inline-flex items-center gap-1.5 ${className}`}>
      <span className={labelClassName}>{label}</span>
      <InfoTip
        text={tooltip}
        className={tipClassName}
        placement={tipPlacement}
        widthClassName={tipWidthClassName}
        panelClassName={tipPanelClassName}
      />
    </span>
  )
}

export default InfoTip
