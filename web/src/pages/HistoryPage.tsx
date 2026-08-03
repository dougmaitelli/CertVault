import type { Job } from "../api/types";
import { formatDate } from "../utils/date";

type HistoryPageProps = {
  jobs: Job[];
};

export function HistoryPage({ jobs }: HistoryPageProps) {
  return (
    <div className="table">
      <table>
        <thead>
          <tr>
            <th>Certificate</th>
            <th>Operation</th>
            <th>Status</th>
            <th>Started</th>
            <th>Result</th>
          </tr>
        </thead>
        <tbody>
          {jobs.map((job) => (
            <tr key={job.id}>
              <td>{job.certificate_name}</td>
              <td>{job.kind}</td>
              <td>
                <span className={`status ${job.status}`}>{job.status}</span>
              </td>
              <td>{formatDate(job.started_at)}</td>
              <td>
                {job.error
                  ? job.error
                  : job.finished_at
                    ? formatDate(job.finished_at)
                    : "Pending"}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
