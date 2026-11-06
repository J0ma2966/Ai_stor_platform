"use client";
import { useEffect, useRef, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Link, useRouter } from "@/i18n/navigation";
import { api, newIdempotencyKey } from "@/lib/api";
import { formatSum } from "@/lib/money";
import { fittingItems } from "@/lib/fitting";
import {
  ANGLES, createGeneration, getQuote, giveConsent, giveTransferConsent, listPhotos, uploadBodyPhoto,
  type Angle, type BodyInput, type Quote, type SavedPhoto,
} from "@/lib/tryon";
import { useFitting } from "@/components/FittingRoom";
import { PhotoGuide, PoseFigure } from "@/components/PhotoGuide";
import { ErrorBox, useErrorText } from "@/components/ErrorBox";

type Slot = { key: string; url: string; saved: boolean } | null;
type Step = "photos" | "body" | "confirm";

const BODY_PARAMS = ["chest_cm", "waist_cm", "hips_cm"] as const;

// Мастер запуска: фото (3 ракурса) → параметры тела (необязательно) → подтверждение.
export function TryOnWizard() {
  const t = useTranslations("tryon");
  const locale = useLocale();
  const router = useRouter();
  const fitting = useFitting();
  const items = fittingItems(fitting);
  const errText = useErrorText();

  const [quote, setQuote] = useState<Quote | null>(null);
  const [step, setStep] = useState<Step>("photos");
  const [slots, setSlots] = useState<Record<Angle, Slot>>({ front: null, side: null, back: null });
  const [savePhotos, setSavePhotos] = useState(true);
  const [consent, setConsent] = useState(false);
  const [transfer, setTransfer] = useState(false);
  const [body, setBody] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState<Angle | "submit" | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [slotErr, setSlotErr] = useState<Partial<Record<Angle, string>>>({});
  const idemKey = useRef(newIdempotencyKey()); // один ключ на попытку запуска — защита от двойного списания

  useEffect(() => {
    (async () => {
      try {
        const [q, saved, me] = await Promise.all([
          getQuote(),
          listPhotos(),
          api<{ height_cm: number | null; weight_kg: number | null; body_params: Record<string, number> }>("/v1/me"),
        ]);
        setQuote(q);
        setConsent(q.consent);
        setTransfer(!q.transfer?.required || q.transfer.consent);
        const s: Record<Angle, Slot> = { front: null, side: null, back: null };
        saved.forEach((p: SavedPhoto) => (s[p.angle] = { key: p.key, url: p.url, saved: true }));
        setSlots(s);
        const b: Record<string, string> = {};
        if (me.height_cm) b.height_cm = String(me.height_cm);
        if (me.weight_kg) b.weight_kg = String(me.weight_kg);
        for (const k of BODY_PARAMS) if (me.body_params?.[k]) b[k] = String(me.body_params[k]);
        setBody(b);
      } catch (e) {
        setError(e);
      }
    })();
  }, []);

  const upload = async (angle: Angle, file: File | undefined) => {
    if (!file) return;
    setSlotErr((e) => ({ ...e, [angle]: undefined }));
    setBusy(angle);
    try {
      const r = await uploadBodyPhoto(file);
      setSlots((s) => ({ ...s, [angle]: { key: r.key, url: r.url, saved: false } }));
      idemKey.current = newIdempotencyKey();
    } catch (e) {
      setSlotErr((s) => ({ ...s, [angle]: errText(e) }));
    } finally {
      setBusy(null);
    }
  };

  const allPhotos = ANGLES.every((a) => slots[a]);

  const toBody = async () => {
    setError(null);
    if (!consent || !transfer) return;
    try {
      if (!quote?.consent) {
        await giveConsent();
        setQuote((q) => (q ? { ...q, consent: true } : q));
      }
      if (quote?.transfer?.required && !quote.transfer.consent) {
        await giveTransferConsent();
        setQuote((q) => (q && q.transfer ? { ...q, transfer: { ...q.transfer, consent: true } } : q));
      }
    } catch (e) {
      setError(e);
      return;
    }
    setStep("body");
  };

  const bodyInput = (): BodyInput | undefined => {
    const num = (k: string) => (body[k] ? Number(body[k].replace(",", ".")) : undefined);
    const params: Record<string, number> = {};
    for (const k of BODY_PARAMS) {
      const v = num(k);
      if (v && v > 0) params[k] = v;
    }
    const out: BodyInput = { height_cm: num("height_cm"), weight_kg: num("weight_kg"), params };
    return out.height_cm || out.weight_kg || Object.keys(params).length ? out : undefined;
  };

  const submit = async () => {
    setBusy("submit");
    setError(null);
    try {
      const g = await createGeneration(
        {
          product_ids: items.map((i) => i.id),
          photos: { front: slots.front!.key, side: slots.side!.key, back: slots.back!.key },
          save_photos: savePhotos && ANGLES.some((a) => !slots[a]?.saved),
          body: bodyInput(),
        },
        idemKey.current,
      );
      router.replace(`/fitting/${g.id}`);
    } catch (e) {
      setError(e);
      setBusy(null);
    }
  };

  if (items.length === 0) {
    return (
      <div className="card stack">
        <p>{t("pickFirst")}</p>
        <Link href="/fitting" className="btn btn-primary">{t("backToRoom")}</Link>
      </div>
    );
  }
  if (!quote) return error ? <ErrorBox error={error} /> : <p className="muted">{t("loading")}</p>;

  if (!quote.registered) {
    return (
      <div className="card stack">
        <p>{t("registrationRequired")}</p>
      </div>
    );
  }

  const stepNames: Step[] = ["photos", "body", "confirm"];
  return (
    <div className="stack">
      <ol className="stepper three">
        {stepNames.map((s, i) => (
          <li key={s} className={`step ${s === step ? "step-current" : stepNames.indexOf(step) > i ? "step-done" : ""}`} aria-current={s === step ? "step" : undefined}>
            <span className="dot">{i + 1}</span>
            {t(`steps.${s}`)}
          </li>
        ))}
      </ol>

      {step === "photos" && (
        <section className="stack">
          <div className="card small">
            <h2>{t("rulesTitle")}</h2>
            <ul>
              {(["rule1", "rule2", "rule3", "rule4"] as const).map((r) => <li key={r}>{t(`rules.${r}`)}</li>)}
            </ul>
            <p className="muted">{t("privacy", { hours: quote.retention.photo_hours, days: quote.retention.saved_photo_days, results: quote.retention.result_days })}</p>
          </div>
          <PhotoGuide defaultOpen={!ANGLES.every((a) => slots[a]?.saved)} />
          <div className="grid-3">
            {ANGLES.map((a) => {
              const s = slots[a];
              return (
                <div key={a} className="card photo-slot">
                  <div className="label">{t(`angle.${a}`)}</div>
                  <p className="small muted">{t(`angleHint.${a}`)}</p>
                  {s ? <img src={s.url} alt={t(`angle.${a}`)} className="photo-preview" /> : <div className="photo-empty" aria-hidden="true"><PoseFigure pose={a} ghost id={`slot-${a}`} /></div>}
                  {s?.saved && <span className="badge badge-info">{t("savedPhoto")}</span>}
                  <label className="btn btn-secondary">
                    {busy === a ? t("uploading") : s ? t("replacePhoto") : t("uploadPhoto")}
                    <input type="file" accept="image/jpeg,image/png,image/webp" className="visually-hidden" disabled={busy !== null}
                      onChange={(e) => upload(a, e.target.files?.[0])} aria-label={t("uploadFor", { angle: t(`angle.${a}`) })} />
                  </label>
                  {s && (
                    <button type="button" className="btn-link small" onClick={() => setSlots((x) => ({ ...x, [a]: null }))}>
                      {t("removePhoto")}
                    </button>
                  )}
                  {slotErr[a] && <p className="field-error" role="alert">{slotErr[a]}</p>}
                </div>
              );
            })}
          </div>
          <label className="row small">
            <input type="checkbox" checked={savePhotos} onChange={(e) => setSavePhotos(e.target.checked)} />
            {t("savePhotos")}
          </label>
          {!quote.consent && (
            <label className="row small">
              <input type="checkbox" checked={consent} onChange={(e) => setConsent(e.target.checked)} />
              {t("consent")}
            </label>
          )}
          {quote.transfer?.required && !quote.transfer.consent && (
            <label className="row small">
              <input type="checkbox" checked={transfer} onChange={(e) => setTransfer(e.target.checked)} />
              {t("transferConsent", { recipient: quote.transfer.recipient })}
            </label>
          )}
          {!allPhotos && <p className="small warn-text">{t("needAllPhotos")}</p>}
          <ErrorBox error={error} />
          <div className="row between">
            <Link href="/fitting" className="btn btn-secondary">{t("backToRoom")}</Link>
            <button type="button" className="btn btn-primary" disabled={!allPhotos || !consent || !transfer || busy !== null} onClick={toBody}>
              {t("next")}
            </button>
          </div>
        </section>
      )}

      {step === "body" && (
        <section className="card stack">
          <p className="small">{t("bodyReminder")}</p>
          <div className="grid-3">
            {(["height_cm", "weight_kg", ...BODY_PARAMS] as const).map((k) => (
              <div key={k} className="field">
                <label htmlFor={`b-${k}`}>{t(`body.${k}`)}</label>
                <input id={`b-${k}`} inputMode="decimal" value={body[k] ?? ""} onChange={(e) => setBody((b) => ({ ...b, [k]: e.target.value.replace(/[^\d.,]/g, "").slice(0, 6) }))} />
              </div>
            ))}
          </div>
          <div className="row between">
            <button type="button" className="btn btn-secondary" onClick={() => setStep("photos")}>{t("back")}</button>
            <button type="button" className="btn btn-primary" onClick={() => setStep("confirm")}>{t("next")}</button>
          </div>
        </section>
      )}

      {step === "confirm" && (
        <section className="card stack">
          <h2>{t("confirmTitle")}</h2>
          <ul className="plain stack">
            {items.map((i) => (
              <li key={i.id} className="row">
                {i.image_url && <img src={i.image_url} alt="" width={48} height={48} className="thumb" />}
                <span className="grow">{t(`zone.${i.zone}`)}: {i.title}</span>
                <span className="muted small">{formatSum(i.price, locale)}</span>
              </li>
            ))}
          </ul>
          <div className="row">
            {ANGLES.map((a) => slots[a] && <img key={a} src={slots[a]!.url} alt={t(`angle.${a}`)} className="thumb-photo" />)}
          </div>
          <p>
            {t("costLine", { cost: quote.cost, balance: quote.balance })}
          </p>
          <p className="small muted">{t("disclaimer")}</p>
          {!quote.enough && (
            <div className="alert">
              <p>{t("notEnough")}</p>
              <Link href="/account/balance">{t("buyTokens")}</Link>
            </div>
          )}
          <ErrorBox error={error} />
          <div className="row between">
            <button type="button" className="btn btn-secondary" onClick={() => setStep("body")} disabled={busy === "submit"}>{t("back")}</button>
            <button type="button" className="btn btn-primary" onClick={submit} disabled={!quote.enough || busy === "submit"}>
              {busy === "submit" ? t("starting") : t("confirmRun", { cost: quote.cost })}
            </button>
          </div>
        </section>
      )}
    </div>
  );
}
