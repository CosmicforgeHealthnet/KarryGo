"use client";

import { useCallback, useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft } from "lucide-react";
import { ApiErrorNotice } from "@/components/features/ApiErrorNotice";
import { FORM_INPUT_CLASS as INPUT, FormField as Field } from "@/components/features/FormField";
import {
  DangerButton,
  KeyValueGrid,
  Loading,
  Notice,
  PageHeader,
  Panel,
  PrimaryButton,
  SecondaryButton,
  StatusPill,
  formatWhen,
  humanize,
} from "@/components/features/OpsUi";
import { describeApiError, newIdempotencyKey, type ApiErrorInfo } from "@/lib/api/errors";
import { incidentService, type IncidentCommand } from "@/lib/services/safetyService";
import { staffService } from "@/lib/services/staffService";
import { ESCALATION_TRIGGERS, INCIDENT_SEVERITIES, type Incident, type Investigation } from "@/lib/types/safety";
import type { StaffMember } from "@/lib/types/staff";

type KeyName = IncidentCommand | "investigate" | "findings" | "approve";

// The stage of an incident: its status, or for an escalated incident the
// status it was escalated from (escalation does not skip the lifecycle).
const stageOf = (i: Incident) => (i.status === "ESCALATED" ? i.preEscalationStatus : i.status);

export default function IncidentDetailPage() {
  const params = useParams();
  const router = useRouter();
  const incidentId = params.incidentId as string;

  const [incident, setIncident] = useState<Incident | null>(null);
  const [investigations, setInvestigations] = useState<Investigation[]>([]);
  const [staff, setStaff] = useState<StaffMember[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiErrorInfo | null>(null);
  const [actionError, setActionError] = useState<ApiErrorInfo | null>(null);
  const [notice, setNotice] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [keys, setKeys] = useState<Record<KeyName, string>>(() => ({
    triage: newIdempotencyKey(),
    contain: newIdempotencyKey(),
    escalate: newIdempotencyKey(),
    resolve: newIdempotencyKey(),
    close: newIdempotencyKey(),
    investigate: newIdempotencyKey(),
    findings: newIdempotencyKey(),
    approve: newIdempotencyKey(),
  }));
  const renew = (k: KeyName) => setKeys((all) => ({ ...all, [k]: newIdempotencyKey() }));

  const [triage, setTriage] = useState({ severity: "", locationReference: "", immediateActions: "" });
  const [containment, setContainment] = useState("");
  const [escalation, setEscalation] = useState({ trigger: "SEVERITY", reason: "" });
  const [investigation, setInvestigation] = useState({ investigatorUserId: "", scope: "", evidencePreservation: "" });
  const [findings, setFindings] = useState({ findings: "", rootCause: "" });
  const [corrective, setCorrective] = useState({ actionType: "", ownerUserId: "", dueAt: "" });
  const [resolution, setResolution] = useState({ action: "", reason: "" });
  const [closeReason, setCloseReason] = useState("");

  const loadInvestigations = useCallback(() => {
    incidentService.investigations(incidentId).then(setInvestigations).catch(() => setInvestigations([]));
  }, [incidentId]);

  const load = useCallback(async () => {
    try {
      const i = await incidentService.get(incidentId);
      setIncident(i);
      setTriage((t) => ({ ...t, severity: t.severity || i.severity, locationReference: t.locationReference || i.locationReference }));
      setError(null);
      loadInvestigations();
    } catch (e) {
      setError(describeApiError(e, "The incident could not be loaded."));
    } finally {
      setLoading(false);
    }
  }, [incidentId, loadInvestigations]);

  useEffect(() => {
    Promise.resolve().then(load);
    staffService.list().then(setStaff).catch(() => setStaff([]));
  }, [load]);

  const command = async (cmd: IncidentCommand, body: Record<string, unknown>, message: string) => {
    if (!incident) return;
    setBusy(true);
    setActionError(null);
    setNotice(null);
    try {
      setIncident(await incidentService.command(incident.id, cmd, body, incident.version, keys[cmd]));
      renew(cmd);
      setNotice(message);
    } catch (e) {
      setActionError(describeApiError(e, "The incident could not be updated."));
    } finally {
      setBusy(false);
    }
  };

  const investigationStep = async (kind: "investigate" | "findings" | "approve", run: () => Promise<void>, message: string) => {
    setBusy(true);
    setActionError(null);
    setNotice(null);
    try {
      await run();
      renew(kind);
      setNotice(message);
      loadInvestigations();
    } catch (e) {
      setActionError(describeApiError(e, "The investigation could not be updated."));
    } finally {
      setBusy(false);
    }
  };

  if (loading) return <Loading label="Loading incident..." />;
  if (!incident) {
    return (
      <div className="space-y-4">
        <ApiErrorNotice error={error} />
        <SecondaryButton onClick={() => router.push("/compliance/incidents")}>Back to incidents</SecondaryButton>
      </div>
    );
  }

  const stage = stageOf(incident);
  const finished = incident.status === "CLOSED";
  const current = investigations.find((v) => v.status !== "APPROVED" && v.status !== "CLOSED");
  const staffName = (id: string) => staff.find((s) => s.id === id)?.fullName ?? id;

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      <PageHeader
        title={`Incident ${incident.id}`}
        description={`${humanize(incident.incidentType)} · occurred ${formatWhen(incident.occurredAt)}`}
        actions={
          <SecondaryButton onClick={() => router.push("/compliance/incidents")}>
            <ArrowLeft className="h-3.5 w-3.5" /> Back
          </SecondaryButton>
        }
      />
      <ApiErrorNotice error={actionError} />
      <Notice message={notice} />

      <KeyValueGrid
        items={[
          ["Status", <StatusPill key="s" status={incident.status} />],
          ["Severity", <StatusPill key="v" status={incident.severity} />],
          ["Location", incident.locationReference || "—"],
          ["Trip", incident.tripReference || "—"],
          ["Triaged", formatWhen(incident.triagedAt)],
          ["Contained", formatWhen(incident.containedAt)],
          ["Escalated", incident.escalatedAt ? `${formatWhen(incident.escalatedAt)} (${humanize(incident.escalationTrigger)})` : "—"],
          ["Resolution", incident.resolutionAction || "—"],
          ["Version", incident.version],
        ]}
      />
      <p className="rounded-xl border border-line bg-white p-4 text-sm text-ink-700">{incident.description}</p>
      {incident.immediateActions && <p className="text-sm text-ink-600">Immediate actions: {incident.immediateActions}</p>}
      {incident.containmentSummary && <p className="text-sm text-ink-600">Containment: {incident.containmentSummary}</p>}

      {!finished && stage === "OPEN" && (
        <Panel title="Triage" description="Confirm severity, location and what was done immediately.">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <Field label="Severity">
              <select value={triage.severity} onChange={(e) => setTriage({ ...triage, severity: e.target.value })} className={INPUT}>
                {INCIDENT_SEVERITIES.map((s) => <option key={s} value={s}>{humanize(s)}</option>)}
              </select>
            </Field>
            <Field label="Location" required>
              <input value={triage.locationReference} onChange={(e) => setTriage({ ...triage, locationReference: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Immediate actions" required>
              <input value={triage.immediateActions} onChange={(e) => setTriage({ ...triage, immediateActions: e.target.value })} placeholder="Area made safe, first aid given" className={INPUT} />
            </Field>
          </div>
          <PrimaryButton
            disabled={busy || !triage.locationReference.trim() || !triage.immediateActions.trim()}
            onClick={() => command("triage", { severity: triage.severity, locationReference: triage.locationReference.trim(), immediateActions: triage.immediateActions.trim() }, "Incident triaged.")}
          >
            Triage incident
          </PrimaryButton>
        </Panel>
      )}

      {!finished && stage === "TRIAGED" && (
        <Panel title="Contain" description="Record how the situation was made safe.">
          <Field label="Containment summary" required>
            <input value={containment} onChange={(e) => setContainment(e.target.value)} className={INPUT} />
          </Field>
          <PrimaryButton disabled={busy || !containment.trim()} onClick={() => command("contain", { containmentSummary: containment.trim() }, "Incident contained.")}>
            Mark contained
          </PrimaryButton>
        </Panel>
      )}

      {!finished && stage === "CONTAINED" && (
        <Panel title="Open investigation">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <Field label="Investigator" required>
              <select value={investigation.investigatorUserId} onChange={(e) => setInvestigation({ ...investigation, investigatorUserId: e.target.value })} className={INPUT}>
                <option value="">Choose a staff member</option>
                {staff.filter((s) => s.status === "ACTIVE").map((s) => <option key={s.id} value={s.id}>{s.fullName}</option>)}
              </select>
            </Field>
            <Field label="Scope" required>
              <input value={investigation.scope} onChange={(e) => setInvestigation({ ...investigation, scope: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Evidence preservation" required>
              <input value={investigation.evidencePreservation} onChange={(e) => setInvestigation({ ...investigation, evidencePreservation: e.target.value })} placeholder="Dashcam footage secured" className={INPUT} />
            </Field>
          </div>
          <PrimaryButton
            disabled={busy || !investigation.investigatorUserId || !investigation.scope.trim() || !investigation.evidencePreservation.trim()}
            onClick={() =>
              investigationStep(
                "investigate",
                async () => {
                  const r = await incidentService.openInvestigation(
                    incident.id,
                    { investigatorUserId: investigation.investigatorUserId, scope: investigation.scope.trim(), evidencePreservation: investigation.evidencePreservation.trim() },
                    incident.version,
                    keys.investigate,
                  );
                  setIncident(r.incident);
                },
                "Investigation opened.",
              )
            }
          >
            Open investigation
          </PrimaryButton>
        </Panel>
      )}

      {current && current.status === "OPEN" && (
        <Panel title="Submit findings" description={`Investigator: ${staffName(current.investigatorUserId)} · scope: ${current.scope}`}>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label="Findings" required>
              <textarea rows={3} value={findings.findings} onChange={(e) => setFindings({ ...findings, findings: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Root cause" required>
              <textarea rows={3} value={findings.rootCause} onChange={(e) => setFindings({ ...findings, rootCause: e.target.value })} className={INPUT} />
            </Field>
          </div>
          <PrimaryButton
            disabled={busy || !findings.findings.trim() || !findings.rootCause.trim()}
            onClick={() =>
              investigationStep(
                "findings",
                async () => {
                  await incidentService.submitFindings(current.id, { findings: findings.findings.trim(), rootCause: findings.rootCause.trim() }, current.version, keys.findings);
                },
                "Findings submitted for approval.",
              )
            }
          >
            Submit findings
          </PrimaryButton>
        </Panel>
      )}

      {current && current.status === "FINDINGS_SUBMITTED" && (
        <Panel title="Approve findings" description={current.findings}>
          <p className="text-sm text-ink-600">Root cause: {current.rootCause}</p>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <Field label="Corrective action" required>
              <input value={corrective.actionType} onChange={(e) => setCorrective({ ...corrective, actionType: e.target.value })} placeholder="DRIVER_RETRAINING" className={INPUT} />
            </Field>
            <Field label="Owner" required>
              <select value={corrective.ownerUserId} onChange={(e) => setCorrective({ ...corrective, ownerUserId: e.target.value })} className={INPUT}>
                <option value="">Choose a staff member</option>
                {staff.filter((s) => s.status === "ACTIVE").map((s) => <option key={s.id} value={s.id}>{s.fullName}</option>)}
              </select>
            </Field>
            <Field label="Due" required>
              <input type="datetime-local" value={corrective.dueAt} onChange={(e) => setCorrective({ ...corrective, dueAt: e.target.value })} className={INPUT} />
            </Field>
          </div>
          <PrimaryButton
            disabled={busy || !corrective.actionType.trim() || !corrective.ownerUserId || !corrective.dueAt}
            onClick={() =>
              investigationStep(
                "approve",
                async () => {
                  const r = await incidentService.approveFindings(
                    current.id,
                    [{ actionType: corrective.actionType.trim(), ownerUserId: corrective.ownerUserId, dueAt: new Date(corrective.dueAt).toISOString() }],
                    current.version,
                    keys.approve,
                  );
                  setIncident(r.incident);
                },
                "Findings approved and corrective action raised.",
              )
            }
          >
            Approve findings
          </PrimaryButton>
        </Panel>
      )}

      {!finished && stage === "CORRECTIVE_ACTION_REQUIRED" && (
        <Panel title="Resolve" description="Record the action taken once corrective actions are done.">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label="Resolution action" required>
              <input value={resolution.action} onChange={(e) => setResolution({ ...resolution, action: e.target.value })} className={INPUT} />
            </Field>
            <Field label="Reason">
              <input value={resolution.reason} onChange={(e) => setResolution({ ...resolution, reason: e.target.value })} className={INPUT} />
            </Field>
          </div>
          <PrimaryButton disabled={busy || !resolution.action.trim()} onClick={() => command("resolve", { resolutionAction: resolution.action.trim(), resolutionReason: resolution.reason.trim() }, "Incident resolved.")}>
            Resolve incident
          </PrimaryButton>
        </Panel>
      )}

      {incident.status === "RESOLVED" && (
        <Panel title="Close">
          <Field label="Closure note">
            <input value={closeReason} onChange={(e) => setCloseReason(e.target.value)} className={INPUT} />
          </Field>
          <PrimaryButton disabled={busy} onClick={() => command("close", { reason: closeReason.trim() }, "Incident closed.")}>
            Close incident
          </PrimaryButton>
        </Panel>
      )}

      {!finished && ["OPEN", "TRIAGED", "CONTAINED", "INVESTIGATING", "CORRECTIVE_ACTION_REQUIRED"].includes(incident.status) && (
        <Panel title="Escalate" description="Escalation keeps the incident at its current stage and flags it for senior attention.">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <Field label="Trigger">
              <select value={escalation.trigger} onChange={(e) => setEscalation({ ...escalation, trigger: e.target.value })} className={INPUT}>
                {ESCALATION_TRIGGERS.map((t) => <option key={t} value={t}>{humanize(t)}</option>)}
              </select>
            </Field>
            <Field label="Reason" required>
              <input value={escalation.reason} onChange={(e) => setEscalation({ ...escalation, reason: e.target.value })} className={INPUT} />
            </Field>
          </div>
          <DangerButton disabled={busy || !escalation.reason.trim()} onClick={() => command("escalate", { trigger: escalation.trigger, reason: escalation.reason.trim() }, "Incident escalated.")}>
            Escalate
          </DangerButton>
        </Panel>
      )}

      <Panel title="Investigations">
        {investigations.length === 0 ? (
          <p className="text-sm text-ink-500">No investigation opened yet.</p>
        ) : (
          <ul className="space-y-3">
            {investigations.map((v) => (
              <li key={v.id} className="rounded-xl border border-line p-3 text-sm">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-mono text-ink-950">{v.id}</span>
                  <StatusPill status={v.status} />
                  <span className="text-xs text-ink-500">opened {formatWhen(v.createdAt)}</span>
                </div>
                <div className="mt-1 text-xs text-ink-600">Investigator {staffName(v.investigatorUserId)} · scope {v.scope}</div>
                {v.findings && <div className="mt-1 text-xs text-ink-700">Findings: {v.findings}</div>}
                {v.rootCause && <div className="text-xs text-ink-700">Root cause: {v.rootCause}</div>}
              </li>
            ))}
          </ul>
        )}
      </Panel>
    </div>
  );
}
