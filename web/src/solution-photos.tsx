import { useEffect, useState } from "react";
import { api, solutionPhotoBlob, SolutionPhoto, uploadSolutionPhoto } from "./api";

export function SolutionPhotoPreview({ photo }: { photo: SolutionPhoto }) {
  const [url, setUrl] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    let active = true, objectUrl = "";
    solutionPhotoBlob(photo.id).then((blob) => {
      if (!active) return;
      objectUrl = URL.createObjectURL(blob); setUrl(objectUrl);
    }).catch((e) => { if (active) setError(e.message); });
    return () => { active = false; if (objectUrl) URL.revokeObjectURL(objectUrl); };
  }, [photo.id]);
  if (error) return <span role="alert">{error}</span>;
  if (!url) return <span>Загружаем фото…</span>;
  return <a href={url} target="_blank" rel="noreferrer" title="Открыть фотографию решения в полном размере">
    <img src={url} alt="Фотография решения ученика" style={{ width: "100%", maxWidth: 240, maxHeight: 220, objectFit: "contain", borderRadius: 8, background: "white" }} />
  </a>;
}

export function SolutionPhotoInput({ attemptId, taskId, photos, onChange, onBusy, required }: {
  attemptId: string; taskId: string; photos: SolutionPhoto[]; onChange: (photos: SolutionPhoto[]) => void; onBusy: (busy: boolean) => void; required: boolean;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const upload = async (files: File[]) => {
    setError("");
    if (photos.length + files.length > 5) { setError("Можно прикрепить до 5 фотографий на задание"); return; }
    if (files.some((f) => !["image/jpeg", "image/png"].includes(f.type) || f.size > 10 * 1024 * 1024)) { setError("Выбери JPEG или PNG, до 10 МБ на фотографию"); return; }
    setBusy(true); onBusy(true);
    const uploaded = [...photos];
    try {
      for (const file of files) {
        uploaded.push(await uploadSolutionPhoto(attemptId, taskId, file));
        onChange([...uploaded]);
      }
    } catch (e) { setError((e as Error).message); }
    finally { setBusy(false); onBusy(false); }
  };
  const remove = async (id: string) => {
    setBusy(true); onBusy(true); setError("");
    try { await api.deleteSolutionPhoto(id); onChange(photos.filter((p) => p.id !== id)); }
    catch (e) { setError((e as Error).message); }
    finally { setBusy(false); onBusy(false); }
  };
  return <div style={{ marginTop: 14, display: "grid", gap: 10 }}>
    <label>Фотографии решения · {required ? "обязательно" : "необязательно"}
      <input type="file" accept="image/jpeg,image/png" multiple disabled={busy || photos.length >= 5} onChange={(e) => { const files = Array.from(e.target.files ?? []); e.target.value = ""; void upload(files); }} style={{ display: "block", marginTop: 8, maxWidth: "100%" }} />
    </label>
    <small style={{ color: "var(--text-3)" }}>До 5 фото JPEG или PNG, до 10 МБ каждое. Сними все шаги решения так, чтобы текст был читаемым.</small>
    {busy && <div role="status">Сохраняем фотографии…</div>}
    {error && <div role="alert" style={{ color: "var(--bad)" }}>{error}</div>}
    <div style={{ display: "flex", flexWrap: "wrap", gap: 12 }}>{photos.map((p, i) => <div key={p.id} style={{ maxWidth: 220 }}>
      <SolutionPhotoPreview photo={p} />
      <button className="btn btn-ghost" disabled={busy} onClick={() => void remove(p.id)}>Удалить фото {i + 1}</button>
    </div>)}</div>
  </div>;
}
