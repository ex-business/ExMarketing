import type { ReactNode } from 'react';

type InspectorSectionProps = {
  title?: ReactNode;
  description?: ReactNode;
  children: ReactNode;
  className?: string;
};

export function InspectorSection({ title, description, children, className = '' }: InspectorSectionProps) {
  return (
    <section className={`exm-inspector-section ${className}`.trim()}>
      {title || description ? (
        <header>
          {title ? <h3>{title}</h3> : null}
          {description ? <p>{description}</p> : null}
        </header>
      ) : null}
      <div className="exm-inspector-section-body">{children}</div>
    </section>
  );
}
