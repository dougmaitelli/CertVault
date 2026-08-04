import { useId } from "react";
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
};

export function MultiSelect({
  label,
  options,
  selected,
  onChange,
}: MultiSelectProps) {
  const labelID = useId();

  function toggle(value: string, checked: boolean) {
    onChange(
      checked
        ? [...selected, value]
        : selected.filter((selectedValue) => selectedValue !== value),
    );
  }

  return (
    <div className="multi-select" role="group" aria-labelledby={labelID}>
      <div className="multi-select-header">
        <span id={labelID}>{label}</span>
        <span className="multi-select-count">{selected.length} selected</span>
      </div>
      <div className="multi-select-options">
        {options.map((option, index) => (
          <Checkbox
            key={option.value}
            checked={selected.includes(option.value)}
            required={index === 0 && selected.length === 0}
            onChange={(event) => toggle(option.value, event.target.checked)}
          >
            {option.label}
          </Checkbox>
        ))}
      </div>
    </div>
  );
}
