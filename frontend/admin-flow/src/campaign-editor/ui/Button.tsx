import type { ButtonHTMLAttributes, ReactNode } from 'react';

type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
type ButtonSize = 'sm' | 'md';

type ButtonProps = ButtonHTMLAttributes<HTMLButtonElement> & {
  variant?: ButtonVariant;
  size?: ButtonSize;
  icon?: ReactNode;
  iconOnly?: boolean;
};

export function Button({
  variant = 'secondary',
  size = 'md',
  icon,
  iconOnly = false,
  className = '',
  children,
  type = 'button',
  ...props
}: ButtonProps) {
  return (
    <button
      {...props}
      type={type}
      className={`exm-button exm-button-${variant} exm-button-${size} ${iconOnly ? 'exm-button-icon-only' : ''} ${className}`.trim()}
    >
      {icon ? <span className="exm-button-icon">{icon}</span> : null}
      {iconOnly ? <span className="exm-sr-only">{children}</span> : children}
    </button>
  );
}
