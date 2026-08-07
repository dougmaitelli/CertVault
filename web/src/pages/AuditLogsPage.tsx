import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { useSearchParams } from "react-router";
import { api } from "../api/client";
import type { AuditPage } from "../api/types";
import { MultiSelect } from "../components/MultiSelect";
import { formatDate } from "../utils/date";
import "./AuditLogsPage.css";

const defaultPerPage = 25;

export function AuditLogsPage() {
  const [params, setParams] = useSearchParams();
  const [result, setResult] = useState<AuditPage>();
  const [error, setError] = useState("");
  const [loadedRequest, setLoadedRequest] = useState("");
  const [search, setSearch] = useState(params.get("q") ?? "");
  const [actors, setActors] = useState(params.getAll("actor"));
  const [actions, setActions] = useState(params.getAll("action"));
  const [resources, setResources] = useState(params.getAll("resource"));

  const page = Math.max(1, Number(params.get("page")) || 1);
  const perPage = Math.max(1, Number(params.get("per_page")) || defaultPerPage);
  const requestParams = new URLSearchParams(params);
  requestParams.set("page", String(page));
  requestParams.set("per_page", String(perPage));
  const request = requestParams.toString();
  const loading = loadedRequest !== request;

  useEffect(() => {
    let active = true;
    api<AuditPage>(`audit?${request}`)
      .then((loaded) => {
        if (!active) return;
        setResult(loaded);
        setError("");
      })
      .catch((caught) => {
        if (active) setError(String(caught));
      })
      .finally(() => {
        if (active) setLoadedRequest(request);
      });
    return () => {
      active = false;
    };
  }, [request]);

  const applyFilters = (event: FormEvent) => {
    event.preventDefault();
    const next = new URLSearchParams();
    if (search.trim()) next.set("q", search.trim());
    actors.forEach((actor) => next.append("actor", actor));
    actions.forEach((action) => next.append("action", action));
    resources.forEach((resource) => next.append("resource", resource));
    if (perPage !== defaultPerPage) next.set("per_page", String(perPage));
    setParams(next);
  };

  const setPage = (nextPage: number) => {
    const next = new URLSearchParams(params);
    next.set("page", String(nextPage));
    setParams(next);
  };

  return (
    <section className="audit-log">
      <form className="audit-filters" onSubmit={applyFilters}>
        <label>
          Search
          <input
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Search audit events"
          />
        </label>
        <MultiSelect
          label="Actors"
          options={(result?.actors ?? actors).map((value) => ({
            label: value,
            value,
          }))}
          selected={actors}
          onChange={setActors}
          required={false}
          compact
        />
        <MultiSelect
          label="Actions"
          options={(result?.actions ?? actions).map((value) => ({
            label: value,
            value,
          }))}
          selected={actions}
          onChange={setActions}
          required={false}
          compact
        />
        <MultiSelect
          label="Resources"
          options={(result?.resources ?? resources).map((value) => ({
            label: value,
            value,
          }))}
          selected={resources}
          onChange={setResources}
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
              <th>Timestamp</th>
              <th>Actor</th>
              <th>Action</th>
              <th>Resource</th>
              <th>Detail</th>
              <th>Source IP</th>
            </tr>
          </thead>
          <tbody>
            {result?.items.map((audit) => (
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
            {!loading && result?.items.length === 0 && (
              <tr>
                <td className="audit-empty" colSpan={6}>
                  No audit events match these filters.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="audit-pagination">
        <span>{loading ? "Loading…" : `${result?.total ?? 0} events`}</span>
        <label>
          Rows{" "}
          <select
            value={perPage}
            onChange={(event) => {
              const next = new URLSearchParams(params);
              next.set("per_page", event.target.value);
              next.delete("page");
              setParams(next);
            }}
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
