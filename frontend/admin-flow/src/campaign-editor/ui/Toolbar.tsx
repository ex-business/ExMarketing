import type { ReactNode } from 'react';

type ToolbarProps = {
  title?: ReactNode;
  description?: ReactNode;
  children?: ReactNode;
  className?: string;
};

export function Toolbar({ title, description, children, className = '' }: ToolbarProps) {
  return (
    <header className={`exm-toolbar ${className}`.trim()}>
      {title || description ? (
        <div className="exm-toolbar-title">
          {title ? <h2>{title}</h2> : null}
          {description ? <p>{description}</p> : null}
        </div>
      ) : null}
      <div className="exm-toolbar-actions">{children}</div>
    </header>
  );
}
