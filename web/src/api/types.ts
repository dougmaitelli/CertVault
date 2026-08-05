export type CertificateVersion = {
  id: number;
  not_before: string;
  not_after: string;
  created_at: string;
  domains: string[];
  serial: string;
  issuer: string;
  fingerprint_sha256: string;
};

export type Certificate = {
  name: string;
  domains: string[];
  key_type: string;
  status: string;
  last_error?: string;
  current_version?: CertificateVersion;
};

export type APIKey = {
  id: number;
  name: string;
  prefix: string;
  scopes: string[];
  certificates: string[];
  created_at: string;
  last_used_at?: string;
  revoked: boolean;
};

export type ACMEAccount = {
  id: string;
  directory_url?: string;
  email: string;
  status: string;
  registration_url?: string;
  current: boolean;
};

export type APIKeyCreationResponse = {
  api_key: APIKey;
  token: string;
};

export type Job = {
  id: number;
  certificate_name: string;
  kind: string;
  status: string;
  error: string;
  started_at: string;
  finished_at?: string;
};

export type Audit = {
  id: number;
  at: string;
  actor: string;
  action: string;
  resource: string;
  detail?: string;
  ip?: string;
};

export type AuditPage = {
  items: Audit[];
  total: number;
  page: number;
  per_page: number;
  total_pages: number;
  actors: string[];
  actions: string[];
  resources: string[];
};

export type Session = {
  name: string;
  email?: string;
  picture?: string;
  authentication_method?: string;
  admin: boolean;
};
