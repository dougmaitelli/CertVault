import { useState } from "react";
import { Modal } from "../components/Modal";
import "./ConfirmationDialog.css";

type ConfirmationDialogProps = {
  title: string;
  message: string;
  confirmLabel: string;
  onClose: () => void;
  onConfirm: () => Promise<void>;
};

export function ConfirmationDialog({
  title,
  message,
  confirmLabel,
  onClose,
  onConfirm,
}: ConfirmationDialogProps) {
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");

  const close = () => {
    if (!pending) onClose();
  };

  async function confirm() {
    setPending(true);
    setError("");
    try {
      await onConfirm();
      onClose();
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : String(caught));
      setPending(false);
    }
  }

  return (
    <Modal className="confirmation-modal" onClose={close}>
      <div className="confirmation-icon" aria-hidden="true">
        !
      </div>
      <div className="confirmation-content">
        <h2>{title}</h2>
        <p>{message}</p>
      </div>
      {error && <div className="error">{error}</div>}
      <div className="confirmation-actions">
        <button className="action-button" disabled={pending} onClick={close}>
          Cancel
        </button>
        <button
          className="action-button delete"
          disabled={pending}
          onClick={() => void confirm()}
        >
          {pending ? "Working…" : confirmLabel}
        </button>
      </div>
    </Modal>
  );
}
