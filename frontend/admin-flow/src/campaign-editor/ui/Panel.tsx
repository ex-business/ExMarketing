import type { ReactNode } from 'react';
import { X } from 'lucide-react';
import { Button } from './Button';

type PanelProps = {
  title: ReactNode;
  children: ReactNode;
  onClose?: () => void;
  footer?: ReactNode;
  className?: string;
  bodyClassName?: string;
};

export function Panel({ title, children, onClose, footer, className = '', bodyClassName = '' }: PanelProps) {
  return (
    <section className={`exm-panel ${className}`.trim()}>
      <header className="exm-panel-head">
        <h2>{title}</h2>
        {onClose ? <Button variant="ghost" icon={<X size={17} />} iconOnly aria-label="关闭" onClick={onClose}>关闭</Button> : null}
      </header>
      <div className={`exm-panel-body ${bodyClassName}`.trim()}>{children}</div>
      {footer ? <footer className="exm-panel-footer">{footer}</footer> : null}
    </section>
  );
}
