import type { ReactNode } from 'react';

export type DataTableColumn<T> = {
  key: string;
  header: ReactNode;
  render: (row: T) => ReactNode;
  className?: string;
};

type DataTableProps<T> = {
  columns: Array<DataTableColumn<T>>;
  rows: T[];
  rowKey: (row: T) => string;
  selectedKey?: string;
  empty?: ReactNode;
  className?: string;
  beforeRows?: ReactNode;
  onRowClick?: (row: T) => void;
};

export function DataTable<T>({ columns, rows, rowKey, selectedKey, empty = '没有数据', className = '', beforeRows, onRowClick }: DataTableProps<T>) {
  return (
    <table className={`exm-data-table ${className}`.trim()}>
      <thead>
        <tr>
          {columns.map((column) => <th key={column.key} className={column.className}>{column.header}</th>)}
        </tr>
      </thead>
      <tbody>
        {beforeRows}
        {rows.map((row) => {
          const key = rowKey(row);
          return (
            <tr key={key} className={selectedKey === key ? 'selected' : ''} onClick={() => onRowClick?.(row)}>
              {columns.map((column) => <td key={column.key} className={column.className}>{column.render(row)}</td>)}
            </tr>
          );
        })}
        {rows.length === 0 && !beforeRows ? <tr><td colSpan={columns.length} className="exm-data-table-empty">{empty}</td></tr> : null}
      </tbody>
    </table>
  );
}
