import { useEffect, useId, useRef } from "react";
import { Checkbox } from "./Checkbox";
import "./MultiSelect.css";

export type MultiSelectOption = {
  label: string;
  value: string;
};

type MultiSelectProps = {
  label: string;
  options: MultiSelectOption[];
  selected: string[];
  onChange: (selected: string[]) => void;
  required?: boolean;
  compact?: boolean;
};

export function MultiSelect({
  label,
  options,
  selected,
  onChange,
  required = true,
  compact = false,
}: MultiSelectProps) {
  const labelID = useId();
  const detailsRef = useRef<HTMLDetailsElement>(null);

  useEffect(() => {
    if (!compact) return;

    const closeOnOutsideClick = (event: PointerEvent) => {
      if (!detailsRef.current?.contains(event.target as Node)) {
        detailsRef.current?.removeAttribute("open");
      }
    };
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || !detailsRef.current?.open) return;
      detailsRef.current.removeAttribute("open");
      detailsRef.current.querySelector("summary")?.focus();
    };

    document.addEventListener("pointerdown", closeOnOutsideClick);
    document.addEventListener("keydown", closeOnEscape);
    return () => {
      document.removeEventListener("pointerdown", closeOnOutsideClick);
      document.removeEventListener("keydown", closeOnEscape);
    };
  }, [compact]);

  function toggle(value: string, checked: boolean) {
    onChange(
      checked
        ? [...selected, value]
        : selected.filter((selectedValue) => selectedValue !== value),
    );
  }

  const header = (
    <>
      <span id={labelID}>{label}</span>
      <span className="multi-select-count">{selected.length} selected</span>
    </>
  );
  const optionsList = (
    <div className="multi-select-options">
      {options.map((option, index) => (
        <Checkbox
          key={option.value}
          checked={selected.includes(option.value)}
          required={required && index === 0 && selected.length === 0}
          onChange={(event) => toggle(option.value, event.target.checked)}
        >
          {option.label}
        </Checkbox>
      ))}
    </div>
  );

  if (compact) {
    return (
      <details ref={detailsRef} className="multi-select compact">
        <summary className="multi-select-header">{header}</summary>
        {optionsList}
      </details>
    );
  }

  return (
    <div className="multi-select" role="group" aria-labelledby={labelID}>
      <div className="multi-select-header">{header}</div>
      {optionsList}
    </div>
  );
}
