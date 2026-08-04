import type { InputHTMLAttributes, ReactNode } from "react";

type CheckboxProps = Omit<InputHTMLAttributes<HTMLInputElement>, "type"> & {
  children: ReactNode;
};

export function Checkbox({ children, className, ...props }: CheckboxProps) {
  const classes = ["checkbox", className].filter(Boolean).join(" ");

  return (
    <label className={classes}>
      <input type="checkbox" {...props} />
      <span className="checkbox-control" aria-hidden="true">
        <svg viewBox="0 0 12 10">
          <path d="m1 5 3 3 7-7" />
        </svg>
      </span>
      <span className="checkbox-label">{children}</span>
    </label>
  );
}
