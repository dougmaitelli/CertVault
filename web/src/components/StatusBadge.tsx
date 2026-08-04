import type { ReactNode } from "react";
import "./StatusBadge.css";

type StatusBadgeProps = {
  status: string;
  label?: string;
  active?: boolean;
};

type StatusBadgeGroupProps = {
  children: ReactNode;
};

export function StatusBadge({ status, label, active }: StatusBadgeProps) {
  return (
    <span className={`status-badge status-badge-${status}`}>
      {active && <i aria-hidden="true" />}
      {label ?? status}
    </span>
  );
}

export function StatusBadgeGroup({ children }: StatusBadgeGroupProps) {
  return <div className="status-badge-group">{children}</div>;
}
