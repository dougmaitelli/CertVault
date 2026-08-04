import type { ReactNode } from "react";
import "./Modal.css";

type ModalProps = {
  children: ReactNode;
  className?: string;
  onClose: () => void;
};

export function Modal({ children, className, onClose }: ModalProps) {
  return (
    <div className="modal" onClick={onClose}>
      <section
        className={className}
        onClick={(event) => event.stopPropagation()}
      >
        <button className="close" onClick={onClose} aria-label="Close">
          ×
        </button>
        {children}
      </section>
    </div>
  );
}
