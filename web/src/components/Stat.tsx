type StatProps = {
  value: number;
  label: string;
};

export function Stat({ value, label }: StatProps) {
  return (
    <div>
      <strong>{value}</strong>
      <span>{label}</span>
    </div>
  );
}
