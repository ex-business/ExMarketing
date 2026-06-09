type BrandMarkProps = {
  compact?: boolean;
};

export function BrandMark({ compact = false }: BrandMarkProps) {
  return (
    <div className={compact ? 'exm-brand-mark compact' : 'exm-brand-mark'} aria-label="ExMarketing">
      <span className="exm-brand-icon" aria-hidden="true">
        <span className="exm-brand-ex">Ex</span>
        <span className="exm-brand-trend" />
        <span className="exm-brand-dot" />
      </span>
      {!compact ? <strong>ExMarketing</strong> : null}
    </div>
  );
}
