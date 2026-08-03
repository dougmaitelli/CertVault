import type { Audit } from "../api/types";
import { formatDate } from "../utils/date";

type AuditLogsPageProps = {
  audits: Audit[];
};

export function AuditLogsPage({ audits }: AuditLogsPageProps) {
  return (
    <div className="table">
      <table>
        <thead>
          <tr>
            <th>Timestamp</th>
            <th>Actor</th>
            <th>Action</th>
            <th>Resource</th>
            <th>Detail</th>
            <th>Source IP</th>
          </tr>
        </thead>
        <tbody>
          {audits.map((audit) => (
            <tr key={audit.id}>
              <td>{formatDate(audit.at)}</td>
              <td>{audit.actor}</td>
              <td>
                <code>{audit.action}</code>
              </td>
              <td>{audit.resource}</td>
              <td>{audit.detail ?? "—"}</td>
              <td>{audit.ip ?? "—"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
