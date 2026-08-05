import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { useSearchParams } from "react-router-dom";
import { api } from "../api/client";
import type { JobPage } from "../api/types";
import { MultiSelect } from "../components/MultiSelect";
import { StatusBadge } from "../components/StatusBadge";
import { formatDate } from "../utils/date";
import "./HistoryPage.css";

const defaultPerPage = 25;

export function HistoryPage() {
  const [params, setParams] = useSearchParams();
  const [result, setResult] = useState<JobPage>();
  const [error, setError] = useState("");
  const [loadedRequest, setLoadedRequest] = useState("");
  const [certificates, setCertificates] = useState(
    params.getAll("certificate"),
  );
  const [operations, setOperations] = useState(params.getAll("operation"));
  const [statuses, setStatuses] = useState(params.getAll("status"));
  const page = Math.max(1, Number(params.get("page")) || 1);
  const perPage = Math.max(1, Number(params.get("per_page")) || defaultPerPage);
  const requestParams = new URLSearchParams(params);
  requestParams.set("page", String(page));
  requestParams.set("per_page", String(perPage));
  const request = requestParams.toString();
  const loading = loadedRequest !== request;

  useEffect(() => {
    let active = true;
    api<JobPage>(`jobs/history?${request}`)
      .then((loaded) => {
        if (active) {
          setResult(loaded);
          setError("");
        }
      })
      .catch((caught) => {
        if (active) {
          setError(String(caught));
        }
      })
      .finally(() => {
        if (active) {
          setLoadedRequest(request);
        }
      });
    return () => {
      active = false;
    };
  }, [request]);

  const applyFilters = (event: FormEvent) => {
    event.preventDefault();
    const next = new URLSearchParams();
    certificates.forEach((value) => next.append("certificate", value));
    operations.forEach((value) => next.append("operation", value));
    statuses.forEach((value) => next.append("status", value));
    if (perPage !== defaultPerPage) next.set("per_page", String(perPage));
    setParams(next);
  };

  const setPage = (nextPage: number) => {
    const next = new URLSearchParams(params);
    next.set("page", String(nextPage));
    setParams(next);
  };

  const setPageSize = (nextPageSize: string) => {
    const next = new URLSearchParams(params);
    next.set("per_page", nextPageSize);
    next.delete("page");
    setParams(next);
  };

  const options = (values: string[]) =>
    values.map((value) => ({ label: value, value }));

  return (
    <section className="history-log">
      <form className="history-filters" onSubmit={applyFilters}>
        <MultiSelect
          label="Certificates"
          options={options(result?.certificates ?? certificates)}
          selected={certificates}
          onChange={setCertificates}
          required={false}
          compact
        />
        <MultiSelect
          label="Operations"
          options={options(result?.operations ?? operations)}
          selected={operations}
          onChange={setOperations}
          required={false}
          compact
        />
        <MultiSelect
          label="Statuses"
          options={options(result?.statuses ?? statuses)}
          selected={statuses}
          onChange={setStatuses}
          required={false}
          compact
        />
        <button className="action-button" type="submit">
          Apply filters
        </button>
      </form>

      {error && <p className="error">{error}</p>}

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
            {result?.items.map((job) => (
              <tr key={job.id}>
                <td>{job.certificate_name}</td>
                <td>{job.kind}</td>
                <td>
                  <StatusBadge status={job.status} />
                </td>
                <td>{formatDate(job.started_at)}</td>
                <td>
                  {job.error ||
                    (job.finished_at ? formatDate(job.finished_at) : "Pending")}
                </td>
              </tr>
            ))}
            {!loading && result?.items.length === 0 && (
              <tr>
                <td className="history-empty" colSpan={5}>
                  No history entries match these filters.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="history-pagination">
        <span>{loading ? "Loading…" : `${result?.total ?? 0} jobs`}</span>
        <label>
          Rows{" "}
          <select
            value={perPage}
            onChange={(event) => setPageSize(event.target.value)}
          >
            <option value="10">10</option>
            <option value="25">25</option>
            <option value="50">50</option>
            <option value="100">100</option>
          </select>
        </label>
        <button
          className="action-button"
          disabled={loading || page <= 1}
          onClick={() => setPage(page - 1)}
        >
          Previous
        </button>
        <span>
          Page {result?.page ?? page} of {Math.max(1, result?.total_pages ?? 1)}
        </span>
        <button
          className="action-button"
          disabled={loading || !result || page >= result.total_pages}
          onClick={() => setPage(page + 1)}
        >
          Next
        </button>
      </div>
    </section>
  );
}
