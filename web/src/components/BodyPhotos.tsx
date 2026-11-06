"use client";
import { useCallback, useEffect, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { useRouter } from "@/i18n/navigation";
import { formatDateTime } from "@/lib/money";
import { ANGLES, deletePhoto, getQuote, giveConsent, listPhotos, revokeConsent, savePhoto, uploadBodyPhoto, type Angle, type SavedPhoto } from "@/lib/tryon";
import { ErrorBox } from "@/components/ErrorBox";
import { PhotoGuide, PoseFigure } from "@/components/PhotoGuide";

// Профиль → сохранённые фото: просмотреть, заменить, удалить (ТЗ 7.2), отозвать согласие.
export function BodyPhotos() {
  const t = useTranslations("tryon");
  const locale = useLocale();
  const router = useRouter();
  const [photos, setPhotos] = useState<SavedPhoto[] | null>(null);
  const [consent, setConsent] = useState<boolean | null>(null);
  const [busy, setBusy] = useState<Angle | null>(null);
  const [error, setError] = useState<unknown>(null);

  const load = useCallback(async () => {
    try {
      const [q, p] = await Promise.all([getQuote(), listPhotos()]);
      setConsent(q.consent);
      setPhotos(p);
    } catch (e) {
      setError(e);
    }
  }, []);
  useEffect(() => {
    load();
  }, [load]);

  const replace = async (angle: Angle, file?: File) => {
    if (!file) return;
    setBusy(angle);
    setError(null);
    try {
      if (!consent) {
        await giveConsent();
        setConsent(true);
      }
      const up = await uploadBodyPhoto(file);
      setPhotos(await savePhoto(angle, up.key));
    } catch (e) {
      setError(e);
    } finally {
      setBusy(null);
    }
  };

  const remove = async (angle: Angle) => {
    setError(null);
    try {
      await deletePhoto(angle);
      setPhotos((p) => p?.filter((x) => x.angle !== angle) ?? null);
    } catch (e) {
      setError(e);
    }
  };

  const revoke = async () => {
    if (!window.confirm(t("revokeConfirm"))) return;
    try {
      await revokeConsent();
      router.refresh();
      await load();
    } catch (e) {
      setError(e);
    }
  };

  if (!photos) return error ? <ErrorBox error={error} onRetry={load} /> : <p className="muted">{t("loading")}</p>;
  return (
    <div className="stack">
      <p className="small muted">{t("photosIntro")}</p>
      <PhotoGuide defaultOpen={photos.length < ANGLES.length} />
      <ErrorBox error={error} />
      <div className="grid-3">
        {ANGLES.map((a) => {
          const p = photos.find((x) => x.angle === a);
          return (
            <div key={a} className="card photo-slot">
              <div className="label">{t(`angle.${a}`)}</div>
              {p ? <img src={p.url} alt={t(`angle.${a}`)} className="photo-preview" /> : <div className="photo-empty" aria-hidden="true"><PoseFigure pose={a} ghost id={`mine-${a}`} /></div>}
              {p && <p className="small muted">{t("photoExpires", { date: formatDateTime(p.expires_at, locale) })}</p>}
              <label className="btn btn-secondary">
                {busy === a ? t("uploading") : p ? t("replacePhoto") : t("uploadPhoto")}
                <input type="file" accept="image/jpeg,image/png,image/webp" className="visually-hidden" disabled={busy !== null}
                  onChange={(e) => replace(a, e.target.files?.[0])} aria-label={t("uploadFor", { angle: t(`angle.${a}`) })} />
              </label>
              {p && <button type="button" className="btn-link small" onClick={() => remove(a)}>{t("removePhoto")}</button>}
            </div>
          );
        })}
      </div>
      {consent && (
        <div className="card stack">
          <p className="small">{t("revokeText")}</p>
          <button type="button" className="btn btn-danger" onClick={revoke}>{t("revoke")}</button>
        </div>
      )}
    </div>
  );
}
