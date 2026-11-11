"use client";
import { useEffect, useState } from "react";
import Link from "next/link";
import { useOnboarding } from "../onboarding-context";
import {
  organizationService as api,
  type WalletProvisioningResponse,
} from "@/lib/services/organizationService";
import { Section, ErrorNotice, Loading, humanize } from "../onboarding-ui";
export default function Wallet() {
  const { data } = useOnboarding();
  const id = data.organization!.organizationId;
  const [state, setState] = useState<WalletProvisioningResponse | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [busy, setBusy] = useState(false);
  async function load() {
    setError(null);
    try {
      setState(await api.getWalletProvisioning(id));
    } catch (err) {
      if ((err as { response?: { status: number } }).response?.status === 404)
        setState({ status: "NOT_REQUESTED", attempts: 0 });
      else setError(err);
    }
  }
  useEffect(() => {
    let active = true;
    api
      .getWalletProvisioning(id)
      .then((s) => {
        if (active) setState(s);
      })
      .catch((err) => {
        if (active) {
          if (err.response?.status === 404)
            setState({ status: "NOT_REQUESTED", attempts: 0 });
          else setError(err);
        }
      });
    return () => {
      active = false;
    };
  }, [id]);
  async function retry() {
    if (busy) return;
    setBusy(true);
    try {
      await api.retryWalletProvisioning(id);
      await load();
    } catch (err) {
      setError(err);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="space-y-5">
      <h1 className="text-2xl font-bold">Wallet provisioning</h1>
      {error ? (
        <ErrorNotice error={error} onRetry={load} />
      ) : !state ? (
        <Loading />
      ) : (
        <Section title="Provisioning status">
          <p role="status">{humanize(state.status)}</p>
          <p className="mt-2 text-sm text-ink-500">
            Attempts: {state.attempts}. Wallet provisioning follows the existing
            company approval requirements and does not block company submission.
          </p>
          {state.status === "FAILED_RETRYABLE" && (
            <button
              className="btn btn-primary mt-4"
              onClick={retry}
              disabled={busy}
            >
              {busy ? "Retrying…" : "Retry provisioning"}
            </button>
          )}
        </Section>
      )}
      <Link className="btn btn-secondary" href="/setup/submit">
        Return to company review
      </Link>
    </div>
  );
}
