import '../models/verification_draft.dart';
import '../models/verification_models.dart';

/// Six demo stages the hub can be in — one per Figma frame.
enum VerificationMockStage {
  notStarted, // 01 — nothing submitted yet
  inProgress, // 01 — some submitted, some pending
  pending,    // 10 — all three submitted, awaiting admin
  verified,   // 11 — admin approved everything
  rejected,   // 12 — admin rejected one step
  restricted, // 14 — account suspended
}

/// Stateful mock for the verification flow. Every mutation recomputes the
/// [AllStatusResponse] so the hub UI reflects changes automatically.
class VerificationMock {
  VerificationMock._();

  // ── Current mutable state ──
  static String _identity = 'pending';
  static String _licence = 'pending';
  static String _face = 'pending';
  static String _overall = 'not_started';
  static String? _rejectionReason;
  static String? _rejectionStep;

  // ── Saved drafts (mock keeps every Figma field) ──
  static VerificationDraft? identityDraft;
  static VerificationDraft? licenceDraft;
  static String? selfiePath;

  // ── Stage helper ──
  static VerificationMockStage get stage {
    if (_overall == 'suspended') return VerificationMockStage.restricted;
    if (_overall == 'rejected') return VerificationMockStage.rejected;
    if (_overall == 'verified') return VerificationMockStage.verified;
    if (_overall == 'pending_review') return VerificationMockStage.pending;
    if (_overall == 'in_progress') return VerificationMockStage.inProgress;
    return VerificationMockStage.notStarted;
  }

  // ── Mutations ──

  static void submitIdentity({
    required String govtIdType,
    required String govtIdNumber,
    required String govtIdFilePath,
    required String profilePhotoFilePath,
    String? fullName,
    DateTime? dateOfBirth,
    String? gender,
  }) {
    identityDraft = (identityDraft ?? const VerificationDraft()).copyWith(
      fullName: fullName,
      dateOfBirth: dateOfBirth,
      gender: gender,
      govtIdType: govtIdType,
      govtIdNumber: govtIdNumber,
      govtIdFilePath: govtIdFilePath,
      profilePhotoFilePath: profilePhotoFilePath,
    );
    _identity = 'submitted';
    _rejectionReason = null;
    _rejectionStep = null;
    _recompute();
  }

  static void submitLicence({
    required String licenceNumber,
    required String expiryYear,
    required String expiryMonth,
    required String licenceFilePath,
    String? licenceType,
    String? issuingState,
  }) {
    licenceDraft = (licenceDraft ?? const VerificationDraft()).copyWith(
      licenceNumber: licenceNumber,
      licenceType: licenceType,
      issuingState: issuingState,
      expiryYear: expiryYear,
      expiryMonth: expiryMonth,
      licenceFilePath: licenceFilePath,
    );
    _licence = 'submitted';
    _rejectionReason = null;
    _rejectionStep = null;
    _recompute();
  }

  static void submitFace({required String selfieFilePath}) {
    selfiePath = selfieFilePath;
    _face = 'submitted';
    _rejectionReason = null;
    _rejectionStep = null;
    _recompute();
  }

  // ── Debug / demo jumps ──

  static void jumpTo(VerificationMockStage s) {
    switch (s) {
      case VerificationMockStage.notStarted:
        _identity = _licence = _face = 'pending';
        _overall = 'not_started';
        _rejectionReason = _rejectionStep = null;
        break;
      case VerificationMockStage.inProgress:
        _identity = 'submitted';
        _licence = _face = 'pending';
        _overall = 'in_progress';
        _rejectionReason = _rejectionStep = null;
        break;
      case VerificationMockStage.pending:
        _identity = _licence = _face = 'submitted';
        _overall = 'pending_review';
        _rejectionReason = _rejectionStep = null;
        break;
      case VerificationMockStage.verified:
        _identity = _licence = _face = 'approved';
        _overall = 'verified';
        _rejectionReason = _rejectionStep = null;
        break;
      case VerificationMockStage.rejected:
        _identity = 'approved';
        _face = 'approved';
        _licence = 'rejected';
        _overall = 'rejected';
        _rejectionStep = 'licence';
        _rejectionReason =
            'The image was unclear. Please upload a clearer photo.';
        break;
      case VerificationMockStage.restricted:
        _identity = 'rejected';
        _licence = _face = 'pending';
        _overall = 'suspended';
        _rejectionStep = 'identity';
        _rejectionReason =
            'Identity could not be verified after multiple attempts.';
        break;
    }
  }

  static void reset() {
    _identity = _licence = _face = 'pending';
    _overall = 'not_started';
    _rejectionReason = _rejectionStep = null;
    identityDraft = null;
    licenceDraft = null;
    selfiePath = null;
  }

  // ── Recompute overall from step states ──
  //
  // The hub switches screens based on _overall:
  //   not_started / in_progress → Figma 01  (checklist with rows)
  //   pending_review            → Figma 10  (all-submitted spinner state)
  //   verified                  → Figma 11
  //   rejected                  → Figma 12
  //   suspended                 → Figma 14
  //
  // Rule: pending_review ONLY fires when all three uploadable steps are
  // submitted (or approved). Submitting one step keeps the hub on 01.
  static void _recompute() {
    if (_overall == 'suspended') return; // sticky

    final statuses = [_identity, _licence, _face];

    // Any rejection → rejected
    if (statuses.contains('rejected')) {
      _overall = 'rejected';
      return;
    }

    // All three approved → verified
    if (statuses.every((s) => s == 'approved')) {
      _overall = 'verified';
      return;
    }

    // All three submitted or approved → pending_review (Figma 10)
    if (statuses.every((s) => s == 'submitted' || s == 'approved')) {
      _overall = 'pending_review';
      return;
    }

    // Some progress but not all → in_progress (Figma 01 with mixed rows)
    final anyProgress = statuses.any((s) => s != 'pending');
    _overall = anyProgress ? 'in_progress' : 'not_started';
  }

  // ── Produce the same shape the API returns ──

  static AllStatusResponse status() {
    final submittedNow = DateTime.now().subtract(const Duration(hours: 2));
    return AllStatusResponse(
      overallStatus: _overall,
      completionPercentage: _computeProgress(),
      steps: [
        VerificationStepSummary(
          step: 'identity',
          status: _identity,
          isOptional: false,
          submittedAt: _identity == 'pending' ? null : submittedNow,
          reviewedAt: _identity == 'approved' ? submittedNow : null,
          rejectionReason:
              _rejectionStep == 'identity' ? _rejectionReason : null,
        ),
        VerificationStepSummary(
          step: 'licence',
          status: _licence,
          isOptional: false,
          submittedAt: _licence == 'pending' ? null : submittedNow,
          reviewedAt: _licence == 'approved' ? submittedNow : null,
          rejectionReason:
              _rejectionStep == 'licence' ? _rejectionReason : null,
          licenceExpiryDate: DateTime.now().add(const Duration(days: 180)),
        ),
        VerificationStepSummary(
          step: 'face',
          status: _face,
          isOptional: false,
          submittedAt: _face == 'pending' ? null : submittedNow,
          reviewedAt: _face == 'approved' ? submittedNow : null,
          rejectionReason: _rejectionStep == 'face' ? _rejectionReason : null,
        ),
        const VerificationStepSummary(
          step: 'vehicle',
          status: 'approved',
          isOptional: false,
        ),
        const VerificationStepSummary(
          step: 'guarantor',
          status: 'approved',
          isOptional: false,
        ),
        const VerificationStepSummary(
          step: 'emergency',
          status: 'approved',
          isOptional: false,
        ),
      ],
    );
  }

  static int _computeProgress() {
    final done = [_identity, _licence, _face]
        .where((s) => s == 'approved' || s == 'submitted')
        .length;
    return ((done / 3) * 100).round();
  }
}