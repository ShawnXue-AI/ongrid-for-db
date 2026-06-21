import { request } from './client';

// Secrets/credentials API client — /v1/secrets/* (HLD-017 credential vault).
// A credential is a NAMED, MULTI-FIELD instance (n8n model). Field VALUES
// are write-only: the list returns only field_keys, never the values.

export interface SecretView {
  id: number;
  name: string;
  description: string;
  field_keys: string[];
  created_at: string;
  updated_at: string;
}

export function listSecrets() {
  return request<{ items: SecretView[] }>('GET', '/secrets');
}

export function createSecret(input: { name: string; description?: string; fields: Record<string, string> }) {
  return request<SecretView>('POST', '/secrets', input);
}

// Update description and/or re-seal fields (omit fields to edit only the note).
export function updateSecret(id: number, input: { description?: string; fields?: Record<string, string> }) {
  return request<{ ok: boolean }>('PUT', `/secrets/${id}`, input);
}

export function deleteSecret(id: number) {
  return request<{ ok: boolean }>('DELETE', `/secrets/${id}`);
}
