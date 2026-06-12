import type { ButtonHTMLAttributes, HTMLAttributes, InputHTMLAttributes, ReactNode, SelectHTMLAttributes } from 'react';

type Tone = 'primary' | 'outline' | 'ghost' | 'danger';

export function TaButton({
  tone = 'outline',
  size = 'md',
  className = '',
  children,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  tone?: Tone;
  size?: 'sm' | 'md';
}) {
  return (
    <button className={`ta-btn ta-btn-${tone} ta-btn-${size} ${className}`.trim()} type="button" {...props}>
      {children}
    </button>
  );
}

export function TaIconButton({
  tone = 'ghost',
  className = '',
  children,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { tone?: Tone }) {
  return (
    <button className={`ta-icon-btn ta-btn-${tone} ${className}`.trim()} type="button" {...props}>
      {children}
    </button>
  );
}

export function TaCard({
  className = '',
  children,
  ...props
}: HTMLAttributes<HTMLElement>) {
  return (
    <section className={`ta-card ${className}`.trim()} {...props}>
      {children}
    </section>
  );
}

export function TaCardHeader({
  title,
  description,
  actions,
}: {
  title: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <header className="ta-card-head">
      <div>
        <h3>{title}</h3>
        {description ? <p>{description}</p> : null}
      </div>
      {actions ? <div className="ta-card-actions">{actions}</div> : null}
    </header>
  );
}

export function TaBadge({
  tone = 'gray',
  children,
  className = '',
}: {
  tone?: 'brand' | 'success' | 'warning' | 'error' | 'gray';
  children: ReactNode;
  className?: string;
}) {
  return <span className={`ta-badge ta-badge-${tone} ${className}`.trim()}>{children}</span>;
}

export function TaNavButton({
  active = false,
  className = '',
  children,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { active?: boolean }) {
  return (
    <button className={`ta-nav-item ${active ? 'active' : ''} ${className}`.trim()} type="button" {...props}>
      {children}
    </button>
  );
}

export function TaInput({
  className = '',
  ...props
}: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={`ta-input ${className}`.trim()} {...props} />;
}

export function TaCheckbox({
  className = '',
  ...props
}: InputHTMLAttributes<HTMLInputElement>) {
  return <input className={`ta-checkbox ${className}`.trim()} type="checkbox" {...props} />;
}

export function TaSelect({
  className = '',
  children,
  ...props
}: SelectHTMLAttributes<HTMLSelectElement>) {
  return <select className={`ta-select ${className}`.trim()} {...props}>{children}</select>;
}

export function TaTabs({
  children,
  className = '',
  ...props
}: HTMLAttributes<HTMLElement>) {
  return <nav className={`ta-tabs ${className}`.trim()} {...props}>{children}</nav>;
}

export function TaTab({
  active = false,
  className = '',
  children,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { active?: boolean }) {
  return (
    <button className={`ta-tab ${active ? 'active' : ''} ${className}`.trim()} type="button" {...props}>
      {children}
    </button>
  );
}

export function TaMenu({
  className = '',
  children,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return <div className={`ta-menu ${className}`.trim()} {...props}>{children}</div>;
}

export function TaMenuItem({
  danger = false,
  className = '',
  children,
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & { danger?: boolean }) {
  return (
    <button className={`ta-menu-item ${danger ? 'danger' : ''} ${className}`.trim()} type="button" {...props}>
      {children}
    </button>
  );
}

export function TaTable({
  className = '',
  children,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return <div className={`ta-table ${className}`.trim()} {...props}>{children}</div>;
}

export function TaTableRow({
  header = false,
  selected = false,
  className = '',
  children,
  ...props
}: HTMLAttributes<HTMLDivElement> & { header?: boolean; selected?: boolean }) {
  return (
    <div className={`ta-table-row ${header ? 'head' : ''} ${selected ? 'selected' : ''} ${className}`.trim()} {...props}>
      {children}
    </div>
  );
}

export function TaEmptyState({
  className = '',
  children,
  ...props
}: HTMLAttributes<HTMLDivElement>) {
  return <div className={`ta-empty ${className}`.trim()} {...props}>{children}</div>;
}

export function TaPagination({
  className = '',
  children,
  ...props
}: HTMLAttributes<HTMLElement>) {
  return <footer className={`ta-pagination ${className}`.trim()} {...props}>{children}</footer>;
}
