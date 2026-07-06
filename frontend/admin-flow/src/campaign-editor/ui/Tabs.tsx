type TabsProps<T extends string> = {
  value: T;
  items: T[];
  onChange: (value: T) => void;
  ariaLabel?: string;
  className?: string;
};

export function Tabs<T extends string>({ value, items, onChange, ariaLabel, className = '' }: TabsProps<T>) {
  return (
    <nav className={`exm-tabs ${className}`.trim()} aria-label={ariaLabel}>
      {items.map((item) => (
        <button key={item} className={item === value ? 'active' : ''} type="button" onClick={() => onChange(item)}>
          {item}
        </button>
      ))}
    </nav>
  );
}
