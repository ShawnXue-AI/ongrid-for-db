import { useCallback, useEffect, useState } from 'react';
import { Lock, Plus, Trash2, RefreshCw, X } from 'lucide-react';
import { listSecrets, createSecret, deleteSecret, type SecretView } from '@/api/secrets';
import { ApiError } from '@/api/client';
import { useI18n } from '@/i18n/locale';

// Settings → Secrets (HLD-017). The generic credential vault. A credential
// is a NAMED, MULTI-FIELD instance (n8n model): "tencent-prod" → {secret_id,
// secret_key, region}. Skills / external MCP declare in their manifest WHERE
// each field injects (env var / file) and a per-skill binding picks WHICH
// instance fills the slot. Field values are write-only — the list shows only
// the field names. Values are AES-encrypted at rest (ONGRID_SECRET_KEY).

type FieldRow = { key: string; value: string };

export default function SecretsPage() {
  const { tr } = useI18n();
  const [items, setItems] = useState<SecretView[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  // add-form state
  const [name, setName] = useState('');
  const [desc, setDesc] = useState('');
  const [rows, setRows] = useState<FieldRow[]>([{ key: '', value: '' }]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const r = await listSecrets();
      setItems(r.items ?? []);
      setErr(null);
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : (e as Error).message);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const setRow = (i: number, patch: Partial<FieldRow>) =>
    setRows((rs) => rs.map((r, idx) => (idx === i ? { ...r, ...patch } : r)));
  const addRow = () => setRows((rs) => [...rs, { key: '', value: '' }]);
  const removeRow = (i: number) => setRows((rs) => (rs.length > 1 ? rs.filter((_, idx) => idx !== i) : rs));

  const onAdd = async () => {
    const fields: Record<string, string> = {};
    for (const r of rows) {
      const k = r.key.trim();
      if (k && r.value) fields[k] = r.value;
    }
    if (!name.trim() || Object.keys(fields).length === 0) return;
    setBusy(true);
    setErr(null);
    try {
      await createSecret({ name: name.trim(), description: desc.trim(), fields });
      setName('');
      setDesc('');
      setRows([{ key: '', value: '' }]);
      await load();
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const onDelete = async (id: number) => {
    setBusy(true);
    try {
      await deleteSecret(id);
      await load();
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : (e as Error).message);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="anim-fade space-y-5">
      <div className="flex items-center gap-2">
        <Lock size={18} className="text-zinc-400" />
        <h1 className="text-lg font-semibold text-zinc-100">{tr('密钥', 'Secrets')}</h1>
        <button
          type="button"
          onClick={() => void load()}
          className="ml-auto inline-flex items-center gap-1.5 rounded-md border border-zinc-700 px-2 py-1 text-[12px] text-zinc-300 hover:bg-zinc-800"
        >
          <RefreshCw size={13} />
          {tr('刷新', 'Refresh')}
        </button>
      </div>

      <p className="text-[13px] leading-relaxed text-zinc-500">
        {tr(
          '凭据库。一份凭据是一个命名实例，含多个字段（如 tencent-prod → secret_id / secret_key / region）。技能 / 外部 MCP 在各自清单里声明每个字段注入到哪（环境变量 / 文件），安装时你只需把它要的凭据槽绑到这里某份凭据。字段值只写不读，AES 加密落库（设 ONGRID_SECRET_KEY）。',
          'Credential vault. A credential is a named instance with multiple fields (e.g. tencent-prod → secret_id / secret_key / region). Skills / external MCP declare in their manifest where each field is injected (env var / file); at install you just bind their credential slot to one of these. Field values are write-only and AES-encrypted at rest (set ONGRID_SECRET_KEY).'
        )}
      </p>

      {err && (
        <div className="rounded-md border border-red-900/50 bg-red-950/30 px-3 py-2 text-[12px] text-red-400">{err}</div>
      )}

      {/* add form */}
      <div className="space-y-2 rounded-lg border border-zinc-800 bg-zinc-900/40 p-3">
        <div className="text-[12px] font-medium text-zinc-300">{tr('新增凭据', 'Add credential')}</div>
        <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={tr('名称（如 tencent-prod）', 'Name (e.g. tencent-prod)')}
            className="rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1.5 font-mono text-[12px] text-zinc-200 outline-none focus:border-zinc-600"
          />
          <input
            value={desc}
            onChange={(e) => setDesc(e.target.value)}
            placeholder={tr('备注（可选）', 'Description (optional)')}
            className="rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1.5 text-[12px] text-zinc-200 outline-none focus:border-zinc-600"
          />
        </div>
        <div className="space-y-1.5">
          <div className="text-[11px] text-zinc-500">{tr('字段（键 / 值）', 'Fields (key / value)')}</div>
          {rows.map((row, i) => (
            <div key={i} className="flex items-center gap-2">
              <input
                value={row.key}
                onChange={(e) => setRow(i, { key: e.target.value })}
                placeholder={tr('字段名（如 secret_id）', 'field key (e.g. secret_id)')}
                className="w-48 rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1.5 font-mono text-[12px] text-zinc-200 outline-none focus:border-zinc-600"
              />
              <input
                value={row.value}
                onChange={(e) => setRow(i, { value: e.target.value })}
                type="password"
                autoComplete="new-password"
                placeholder={tr('值', 'value')}
                className="flex-1 rounded-md border border-zinc-700 bg-zinc-950 px-2 py-1.5 text-[12px] text-zinc-200 outline-none focus:border-zinc-600"
              />
              <button
                type="button"
                onClick={() => removeRow(i)}
                className="rounded border border-zinc-700 p-1.5 text-zinc-500 hover:text-zinc-300"
                title={tr('删除字段', 'Remove field')}
              >
                <X size={13} />
              </button>
            </div>
          ))}
          <button
            type="button"
            onClick={addRow}
            className="inline-flex items-center gap-1 text-[11px] text-indigo-400 hover:text-indigo-300"
          >
            <Plus size={12} />
            {tr('加字段', 'Add field')}
          </button>
        </div>
        <div>
          <button
            type="button"
            onClick={() => void onAdd()}
            disabled={busy || !name.trim()}
            className="inline-flex items-center gap-1.5 rounded-md bg-indigo-600 px-3 py-1.5 text-[12px] font-medium text-white hover:bg-indigo-500 disabled:opacity-40"
          >
            <Plus size={13} />
            {tr('保存凭据', 'Save credential')}
          </button>
        </div>
      </div>

      {/* list */}
      <div className="overflow-hidden rounded-lg border border-zinc-800">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b border-zinc-800 bg-zinc-900/40 text-left text-[11px] uppercase tracking-wide text-zinc-500">
              <th className="px-3 py-2 font-medium">{tr('名称', 'Name')}</th>
              <th className="px-3 py-2 font-medium">{tr('备注', 'Description')}</th>
              <th className="px-3 py-2 font-medium">{tr('字段', 'Fields')}</th>
              <th className="px-3 py-2" />
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <tr>
                <td colSpan={4} className="px-3 py-6 text-center text-[12px] text-zinc-500">
                  {tr('加载中…', 'Loading…')}
                </td>
              </tr>
            ) : items.length === 0 ? (
              <tr>
                <td colSpan={4} className="px-3 py-6 text-center text-[12px] text-zinc-600">
                  {tr('还没有凭据。安装需要凭据的技能时在这里填。', 'No credentials yet. Add them here when installing a skill that needs them.')}
                </td>
              </tr>
            ) : (
              items.map((s) => (
                <tr key={s.id} className="border-b border-zinc-800/60 last:border-0">
                  <td className="px-3 py-2 font-mono text-[12px] text-zinc-200">{s.name}</td>
                  <td className="px-3 py-2 text-[12px] text-zinc-400">
                    {s.description || <span className="text-zinc-600">—</span>}
                  </td>
                  <td className="px-3 py-2">
                    <div className="flex flex-wrap gap-1">
                      {s.field_keys.length === 0 ? (
                        <span className="text-[11px] text-zinc-600">—</span>
                      ) : (
                        s.field_keys.map((k) => (
                          <span key={k} className="rounded bg-zinc-800 px-1.5 py-0.5 font-mono text-[11px] text-zinc-400">
                            {k}
                          </span>
                        ))
                      )}
                    </div>
                  </td>
                  <td className="px-3 py-2 text-right">
                    <button
                      type="button"
                      onClick={() => void onDelete(s.id)}
                      disabled={busy}
                      className="rounded border border-zinc-700 p-1 text-zinc-500 hover:border-red-800 hover:text-red-400 disabled:opacity-40"
                      title={tr('删除', 'Delete')}
                    >
                      <Trash2 size={13} />
                    </button>
                  </td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
