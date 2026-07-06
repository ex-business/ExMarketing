import type { ReactNode } from 'react';

type FieldProps = {
  label?: ReactNode;
  hint?: ReactNode;
  required?: boolean;
  children: ReactNode;
  className?: string;
};

export function Field({ label, hint, required = false, children, className = '' }: FieldProps) {
  return (
    <label className={`exm-field ${className}`.trim()}>
      {label ? (
        <span className="exm-field-label">
          {label}
          {required ? <b> *</b> : null}
        </span>
      ) : null}
      {children}
      {hint ? <small>{hint}</small> : null}
    </label>
  );
}
