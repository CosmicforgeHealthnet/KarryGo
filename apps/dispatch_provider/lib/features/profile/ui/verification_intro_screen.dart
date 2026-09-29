import 'dart:async';

import 'package:flutter/material.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../vehicle/state/vehicle_controller.dart';
import '../../vehicle/ui/bike_list_screen.dart';
import '../../verification/models/verification_models.dart';
import '../../verification/state/verification_controller.dart';
import 'face_intro_screen.dart';
import 'identity_intro_screen.dart';
import 'licence_details_screen.dart';
import 'licence_upload_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 01 · 10 · 11 · 12 · 14 — Verification hub
// One screen, five visual states driven by latestStatus.overallStatus.
//
// Figma shows FOUR rows: Identity, Selfie, Licence, Review.
// Guarantor / Emergency / Vehicle are collected during onboarding and are
// auto-approved on the backend — they are not rider tasks here.
//
// Dependency: the backend refuses POST /verification/face unless the
// identity step is already submitted or approved. The Selfie row is
// therefore disabled until that precondition is met.
// ═══════════════════════════════════════════════════════════════════════════

class VerificationIntroScreen extends StatefulWidget {
  const VerificationIntroScreen({
    super.key,
    required this.verificationController,
    this.vehicleController,
  });

  final VerificationController verificationController;
  final VehicleController? vehicleController;

  @override
  State<VerificationIntroScreen> createState() =>
      _VerificationIntroScreenState();
}

class _VerificationIntroScreenState extends State<VerificationIntroScreen> {
  bool _isLoading = true;
  String? _loadError;
  Timer? _pollTimer;

  @override
  void initState() {
    super.initState();
    _load();
    _pollTimer = Timer.periodic(const Duration(seconds: 60), (_) {
      if (mounted) widget.verificationController.refreshStatus();
    });
  }

  @override
  void dispose() {
    _pollTimer?.cancel();
    super.dispose();
  }

  Future<void> _load() async {
    setState(() {
      _isLoading = true;
      _loadError = null;
    });
    final result =
        await widget.verificationController.loadVerificationStatus();
    if (!mounted) return;
    result.when(
      success: (_) => setState(() => _isLoading = false),
      failure: (error) => setState(() {
        _isLoading = false;
        _loadError = error.code == ApiErrorCode.network
            ? 'Cannot connect to Cosmicforge Logistics server.'
            : error.message.isNotEmpty
                ? error.message
                : 'Could not load verification status.';
      }),
    );
  }

  // ── Row taps ──────────────────────────────────────────────────────────

  Future<void> _openIdentity() async {
    final r = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => IdentityIntroScreen(
          verificationController: widget.verificationController,
        ),
      ),
    );
    if (!mounted) return;
    if (r == true) {
      widget.verificationController.refreshStatus();
      setState(() {});
    }
  }

  Future<void> _openFace() async {
    final r = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => FaceIntroScreen(
          verificationController: widget.verificationController,
        ),
      ),
    );
    if (!mounted) return;
    if (r == true) {
      widget.verificationController.refreshStatus();
      setState(() {});
    }
  }

  Future<void> _openLicence() async {
    final r = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => LicenceDetailsScreen(
          verificationController: widget.verificationController,
        ),
      ),
    );
    if (!mounted) return;
    if (r == true) {
      widget.verificationController.refreshStatus();
      setState(() {});
    }
  }

  Future<void> _openLicenceResubmit() async {
    final r = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => LicenceUploadScreen(
          verificationController: widget.verificationController,
          resubmission: true,
        ),
      ),
    );
    if (!mounted) return;
    if (r == true) {
      widget.verificationController.refreshStatus();
      setState(() {});
    }
  }

  Future<void> _openVehicle() async {
    final vc = widget.vehicleController;
    if (vc == null) return;
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => BikeListScreen(vehicleController: vc),
      ),
    );
    if (!mounted) return;
    widget.verificationController.refreshStatus();
    setState(() {});
  }

  void _openHelp() {
    showDialog<void>(
      context: context,
      builder: (ctx) {
        final c = ctx.palette;
        return AlertDialog(
          title: const Text('Need help?'),
          content: Text(
            'If a step is stuck or you believe a decision was made in error, '
            'contact Karryam support from your profile.',
            style: TextStyle(color: c.textMuted, fontSize: 13, height: 1.5),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: const Text('Got it'),
            ),
          ],
        );
      },
    );
  }

  // ── Build ─────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            _TopBar(onBack: () => Navigator.pop(context), onHelp: _openHelp),
            Expanded(
              child: _isLoading
                  ? Center(
                      child: CircularProgressIndicator(
                        color: c.brandTeal500,
                      ),
                    )
                  : _loadError != null
                      ? _ErrorView(message: _loadError!, onRetry: _load)
                      : ListenableBuilder(
                          listenable: widget.verificationController,
                          builder: (context, _) {
                            final status =
                                widget.verificationController.latestStatus;
                            if (status == null) {
                              return Center(
                                child: CircularProgressIndicator(
                                  color: c.brandTeal500,
                                ),
                              );
                            }
                            return _HubBody(
                              status: status,
                              onOpenIdentity: _openIdentity,
                              onOpenFace: _openFace,
                              onOpenLicence: _openLicence,
                              onOpenLicenceResubmit: _openLicenceResubmit,
                              onOpenVehicle: widget.vehicleController != null
                                  ? _openVehicle
                                  : null,
                              onHelp: _openHelp,
                            );
                          },
                        ),
            ),
          ],
        ),
      ),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Top bar
// ═══════════════════════════════════════════════════════════════════════════

class _TopBar extends StatelessWidget {
  const _TopBar({required this.onBack, required this.onHelp});
  final VoidCallback onBack;
  final VoidCallback onHelp;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
      child: Row(
        children: [
          GestureDetector(
            onTap: onBack,
            behavior: HitTestBehavior.opaque,
            child: Padding(
              padding: const EdgeInsets.all(8),
              child: Icon(
                Icons.arrow_back_rounded,
                size: 22,
                color: c.text,
              ),
            ),
          ),
          const Spacer(),
          GestureDetector(
            onTap: onHelp,
            behavior: HitTestBehavior.opaque,
            child: Padding(
              padding: const EdgeInsets.all(8),
              child: Text(
                'Help',
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w600,
                  color: c.brandTeal500,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Hub body — branches on overallStatus
// ═══════════════════════════════════════════════════════════════════════════

class _HubBody extends StatelessWidget {
  const _HubBody({
    required this.status,
    required this.onOpenIdentity,
    required this.onOpenFace,
    required this.onOpenLicence,
    required this.onOpenLicenceResubmit,
    this.onOpenVehicle,
    required this.onHelp,
  });

  final AllStatusResponse status;
  final VoidCallback onOpenIdentity;
  final VoidCallback onOpenFace;
  final VoidCallback onOpenLicence;
  final VoidCallback onOpenLicenceResubmit;
  final VoidCallback? onOpenVehicle;
  final VoidCallback onHelp;

  @override
  Widget build(BuildContext context) {
    switch (status.overallStatus) {
      case 'verified':
        return const _VerifiedBody();
      case 'pending_review':
        return _PendingBody(status: status);
      case 'rejected':
        return _RejectedBody(
          status: status,
          onResubmit: _resubmitCallback(status),
        );
      case 'suspended':
        return _RestrictedBody(onHelp: onHelp);
      default:
        return _StepsBody(
          status: status,
          onOpenIdentity: onOpenIdentity,
          onOpenFace: onOpenFace,
          onOpenLicence: onOpenLicence,
          onOpenVehicle: onOpenVehicle,
        );
    }
  }

  VoidCallback _resubmitCallback(AllStatusResponse s) {
    final rejected = s.steps.firstWhere(
      (step) => step.status == 'rejected',
      orElse: () => const VerificationStepSummary(
        step: 'identity',
        status: 'rejected',
        isOptional: false,
      ),
    );
    return switch (rejected.step) {
      'face' => onOpenFace,
      'licence' => onOpenLicenceResubmit,
      _ => onOpenIdentity,
    };
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Figma 01 — Steps body (not_started / in_progress)
// ═══════════════════════════════════════════════════════════════════════════

class _StepsBody extends StatelessWidget {
  const _StepsBody({
    required this.status,
    required this.onOpenIdentity,
    required this.onOpenFace,
    required this.onOpenLicence,
    this.onOpenVehicle,
  });

  final AllStatusResponse status;
  final VoidCallback onOpenIdentity;
  final VoidCallback onOpenFace;
  final VoidCallback onOpenLicence;
  final VoidCallback? onOpenVehicle;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final stepMap = {for (final s in status.steps) s.step: s};

    final identity = stepMap['identity'];
    final licence = stepMap['licence'];
    final face = stepMap['face'];
    final review = _aggregateReview(identity, licence, face);

    // The backend refuses POST /verification/face unless identity is
    // submitted or approved. Mirror that gate in the UI.
    final identityReady = identity?.status == 'submitted' ||
        identity?.status == 'approved';

    final rowsDone = [identity, licence, face]
        .where((s) => s?.status == 'approved' || s?.status == 'submitted')
        .length;
    final reviewDone = review.status == 'approved' ||
        review.status == 'submitted';
    final totalDone = rowsDone + (reviewDone ? 1 : 0);
    final pct = ((totalDone / 4) * 100).round();

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Expanded(
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 8, 20, 12),
            children: [
              Text(
                'Complete your verification',
                style: TextStyle(
                  fontSize: 26,
                  fontWeight: FontWeight.w800,
                  color: c.text,
                  letterSpacing: -0.4,
                ),
              ),
              const SizedBox(height: 6),
              Text(
                'Verify your identity to unlock more opportunities on Karryam.',
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w500,
                  color: c.textMuted,
                  height: 1.45,
                ),
              ),
              const SizedBox(height: 24),
              _ProgressCard(
                done: totalDone,
                total: 4,
                pct: pct,
              ),
              const SizedBox(height: 16),

              // ── Identity ─────────────────────────────────────────
              _StepCard(
                icon: Icons.verified_user_outlined,
                label: 'Identity (BVN)',
                subLabel: 'Government-issued ID',
                status: identity?.status ?? 'pending',
                onTap: onOpenIdentity,
              ),
              const SizedBox(height: 10),

              // ── Selfie (gated on identity) ───────────────────────
              _StepCard(
                icon: Icons.face_retouching_natural,
                label: 'Selfie (Face verification)',
                subLabel: identityReady
                    ? 'Match your face to your ID'
                    : 'Complete identity first',
                status: face?.status ?? 'pending',
                onTap: identityReady ? onOpenFace : null,
              ),
              const SizedBox(height: 10),

              // ── Licence ──────────────────────────────────────────
              _StepCard(
                icon: Icons.badge_outlined,
                label: "Driver's licence",
                subLabel: 'Upload your licence',
                status: licence?.status ?? 'pending',
                onTap: onOpenLicence,
              ),
              const SizedBox(height: 10),

              // ── Review ───────────────────────────────────────────
              _StepCard(
                icon: Icons.description_outlined,
                label: 'Review & submission',
                subLabel: review.status == 'approved'
                    ? 'Submitted to admin'
                    : 'Submit all documents',
                status: review.status,
                onTap: null,
              ),
            ],
          ),
        ),
        _BottomCta(
          label: _ctaLabel(identity, licence, face),
          onPressed: _ctaCallback(identity, licence, face),
        ),
      ],
    );
  }

  static VerificationStepSummary _aggregateReview(
    VerificationStepSummary? identity,
    VerificationStepSummary? licence,
    VerificationStepSummary? face,
  ) {
    final statuses = [
      identity?.status ?? 'pending',
      licence?.status ?? 'pending',
      face?.status ?? 'pending',
    ];
    if (statuses.contains('rejected')) {
      return const VerificationStepSummary(
        step: 'review',
        status: 'rejected',
        isOptional: false,
      );
    }
    if (statuses.every((s) => s == 'approved')) {
      return const VerificationStepSummary(
        step: 'review',
        status: 'approved',
        isOptional: false,
      );
    }
    if (statuses.every((s) => s != 'pending')) {
      return const VerificationStepSummary(
        step: 'review',
        status: 'submitted',
        isOptional: false,
      );
    }
    return const VerificationStepSummary(
      step: 'review',
      status: 'pending',
      isOptional: false,
    );
  }

  String _ctaLabel(
    VerificationStepSummary? identity,
    VerificationStepSummary? licence,
    VerificationStepSummary? face,
  ) {
    final i = identity?.status ?? 'pending';
    final l = licence?.status ?? 'pending';
    final f = face?.status ?? 'pending';

    if (i == 'rejected' || l == 'rejected') return 'Re-submit Documents';
    if (f == 'rejected') return 'Re-submit Selfie';
    if (i == 'pending') return 'Continue';
    if (f == 'pending') return 'Continue';
    if (l == 'pending') return 'Continue';
    if (onOpenVehicle != null) return 'Complete Vehicle Registration';
    return 'Continue';
  }

  VoidCallback? _ctaCallback(
    VerificationStepSummary? identity,
    VerificationStepSummary? licence,
    VerificationStepSummary? face,
  ) {
    final i = identity?.status ?? 'pending';
    final l = licence?.status ?? 'pending';
    final f = face?.status ?? 'pending';

    if (i == 'rejected' || l == 'rejected') return onOpenIdentity;
    if (f == 'rejected') return onOpenFace;
    if (i == 'pending') return onOpenIdentity;
    if (f == 'pending') return onOpenFace;
    if (l == 'pending') return onOpenLicence;
    if (onOpenVehicle != null) return onOpenVehicle;
    return null;
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Figma 10 — Pending
// ═══════════════════════════════════════════════════════════════════════════

class _PendingBody extends StatelessWidget {
  const _PendingBody({required this.status});
  final AllStatusResponse status;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final stepMap = {for (final s in status.steps) s.step: s};

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Expanded(
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 16, 20, 12),
            children: [
              const SizedBox(height: 8),

              Center(
                child: Container(
                  width: 88,
                  height: 88,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: c.info.withValues(alpha: 0.12),
                  ),
                  child: Icon(
                    Icons.schedule_rounded,
                    size: 44,
                    color: c.info,
                  ),
                ),
              ),
              const SizedBox(height: 24),

              Text(
                'Verification in progress',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.w800,
                  color: c.text,
                  letterSpacing: -0.4,
                ),
              ),
              const SizedBox(height: 10),

              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 8),
                child: Text(
                  "We're reviewing your information. This usually takes a few minutes, but can take up to 24 hours.",
                  textAlign: TextAlign.center,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w500,
                    color: c.textMuted,
                    height: 1.5,
                  ),
                ),
              ),
              const SizedBox(height: 24),

              _ProgressDots(
                count: 4,
                filled: ['identity', 'licence', 'face']
                    .where((k) => (stepMap[k]?.status ?? 'pending') != 'pending')
                    .length,
              ),
              const SizedBox(height: 28),

              _CheckRow(
                label: 'Identity (BVN)',
                status: stepMap['identity']?.status ?? 'pending',
              ),
              const SizedBox(height: 16),
              _CheckRow(
                label: 'Selfie (Face verification)',
                status: stepMap['face']?.status ?? 'pending',
              ),
              const SizedBox(height: 16),
              _CheckRow(
                label: "Driver's licence",
                status: stepMap['licence']?.status ?? 'pending',
              ),
              const SizedBox(height: 16),
              const _CheckRow(
                label: 'Review & submission',
                status: 'submitted',
              ),
              const SizedBox(height: 28),

              Center(
                child: Text(
                  "We'll notify you as soon as verification is complete.",
                  textAlign: TextAlign.center,
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w500,
                    color: c.textMuted,
                    height: 1.5,
                  ),
                ),
              ),
            ],
          ),
        ),
        _BottomCta(
          label: 'Continue to dashboard',
          onPressed: () => Navigator.of(context).pop(false),
        ),
      ],
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Figma 11 — Verified
// ═══════════════════════════════════════════════════════════════════════════

class _VerifiedBody extends StatelessWidget {
  const _VerifiedBody();

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Expanded(
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 16, 20, 12),
            children: [
              const SizedBox(height: 8),
              Center(
                child: Container(
                  width: 88,
                  height: 88,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: c.successSoft,
                  ),
                  child: Icon(
                    Icons.check_rounded,
                    size: 52,
                    color: c.success,
                  ),
                ),
              ),
              const SizedBox(height: 24),
              Text(
                'Verified!',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.w800,
                  color: c.text,
                  letterSpacing: -0.4,
                ),
              ),
              const SizedBox(height: 10),
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 8),
                child: Text(
                  "Your identity, selfie and driver's licence have been successfully verified.",
                  textAlign: TextAlign.center,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w500,
                    color: c.textMuted,
                    height: 1.5,
                  ),
                ),
              ),
              const SizedBox(height: 28),
              const _CheckRow(label: 'Identity (BVN)', status: 'approved'),
              const SizedBox(height: 16),
              const _CheckRow(
                label: 'Selfie (Face verification)',
                status: 'approved',
              ),
              const SizedBox(height: 16),
              const _CheckRow(
                label: "Driver's licence",
                status: 'approved',
              ),
              const SizedBox(height: 16),
              const _CheckRow(
                label: 'Review & submission',
                status: 'approved',
              ),
            ],
          ),
        ),
        _BottomCta(
          label: 'Continue to dashboard',
          onPressed: () => Navigator.of(context).pop(false),
        ),
      ],
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Figma 12 — Rejected
// ═══════════════════════════════════════════════════════════════════════════

class _RejectedBody extends StatelessWidget {
  const _RejectedBody({required this.status, required this.onResubmit});
  final AllStatusResponse status;
  final VoidCallback onResubmit;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final stepMap = {for (final s in status.steps) s.step: s};

    final rejectedStep = status.steps.firstWhere(
      (s) => s.status == 'rejected',
      orElse: () => const VerificationStepSummary(
        step: 'identity',
        status: 'rejected',
        isOptional: false,
      ),
    );

    final stepLabel = switch (rejectedStep.step) {
      'licence' => "driver's licence",
      'face' => 'selfie',
      _ => 'identity',
    };

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Expanded(
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 16, 20, 12),
            children: [
              const SizedBox(height: 8),
              Center(
                child: Container(
                  width: 88,
                  height: 88,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: c.dangerSoft,
                  ),
                  child: Icon(
                    Icons.priority_high_rounded,
                    size: 44,
                    color: c.danger,
                  ),
                ),
              ),
              const SizedBox(height: 24),
              Text(
                'Action required',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.w800,
                  color: c.text,
                  letterSpacing: -0.4,
                ),
              ),
              const SizedBox(height: 10),
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 8),
                child: Text(
                  rejectedStep.rejectionReason?.isNotEmpty == true
                      ? "We couldn't verify your $stepLabel because ${_lowerFirst(rejectedStep.rejectionReason!)}"
                      : "We couldn't verify your $stepLabel. Please resubmit.",
                  textAlign: TextAlign.center,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w500,
                    color: c.textMuted,
                    height: 1.5,
                  ),
                ),
              ),
              const SizedBox(height: 28),
              _CheckRow(
                label: 'Identity (BVN)',
                status: stepMap['identity']?.status ?? 'pending',
              ),
              const SizedBox(height: 16),
              _CheckRow(
                label: 'Selfie (Face verification)',
                status: stepMap['face']?.status ?? 'pending',
              ),
              const SizedBox(height: 16),
              _CheckRow(
                label: "Driver's licence",
                status: stepMap['licence']?.status ?? 'pending',
              ),
              const SizedBox(height: 16),
              const _CheckRow(
                label: 'Review & submission',
                status: 'pending',
              ),
            ],
          ),
        ),
        _BottomCta(
          label: switch (rejectedStep.step) {
            'face' => 'Resubmit selfie',
            'licence' => 'Resubmit licence',
            _ => 'Resubmit documents',
          },
          onPressed: onResubmit,
        ),
      ],
    );
  }

  static String _lowerFirst(String s) {
    if (s.isEmpty) return s;
    return s[0].toLowerCase() + s.substring(1);
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Figma 14 — Restricted
// ═══════════════════════════════════════════════════════════════════════════

class _RestrictedBody extends StatelessWidget {
  const _RestrictedBody({required this.onHelp});
  final VoidCallback onHelp;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Expanded(
          child: ListView(
            padding: const EdgeInsets.fromLTRB(20, 16, 20, 12),
            children: [
              const SizedBox(height: 8),
              Center(
                child: Container(
                  width: 88,
                  height: 88,
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: c.dangerSoft,
                  ),
                  child: Icon(
                    Icons.lock_outline_rounded,
                    size: 40,
                    color: c.danger,
                  ),
                ),
              ),
              const SizedBox(height: 24),
              Text(
                'Account restricted',
                textAlign: TextAlign.center,
                style: TextStyle(
                  fontSize: 24,
                  fontWeight: FontWeight.w800,
                  color: c.text,
                  letterSpacing: -0.4,
                ),
              ),
              const SizedBox(height: 10),
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 8),
                child: Text(
                  'Your account has been restricted because your identity could not be verified after multiple attempts.',
                  textAlign: TextAlign.center,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w500,
                    color: c.textMuted,
                    height: 1.5,
                  ),
                ),
              ),
              const SizedBox(height: 28),
              Text(
                'What this means',
                style: TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w700,
                  color: c.text,
                ),
              ),
              const SizedBox(height: 14),
              const _BulletRow(text: "You can't accept new dispatches"),
              const SizedBox(height: 12),
              const _BulletRow(text: 'Your account is under review'),
              const SizedBox(height: 12),
              const _BulletRow(
                text: 'Contact support if you believe this is a mistake',
              ),
            ],
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 0, 20, 8),
          child: SizedBox(
            height: 52,
            width: double.infinity,
            child: FilledButton(
              onPressed: onHelp,
              style: FilledButton.styleFrom(
                backgroundColor: c.brandNavy900,
                foregroundColor: c.brandWhite,
                shape: const StadiumBorder(),
                textStyle: const TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w700,
                ),
              ),
              child: const Text('Contact support'),
            ),
          ),
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
          child: SizedBox(
            height: 44,
            width: double.infinity,
            child: TextButton(
              onPressed: () => Navigator.of(context).pop(),
              style: TextButton.styleFrom(
                foregroundColor: c.text,
                textStyle: const TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w700,
                ),
              ),
              child: const Text('Log out'),
            ),
          ),
        ),
      ],
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Shared widgets
// ═══════════════════════════════════════════════════════════════════════════

class _ProgressCard extends StatelessWidget {
  const _ProgressCard({
    required this.done,
    required this.total,
    required this.pct,
  });

  final int done;
  final int total;
  final int pct;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Container(
      padding: const EdgeInsets.fromLTRB(14, 14, 14, 14),
      decoration: BoxDecoration(
        color: c.surface,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: c.divider),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  '$done of $total steps completed',
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                    color: c.textMuted,
                  ),
                ),
              ),
              Text(
                '$pct%',
                style: TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w800,
                  color: c.text,
                ),
              ),
            ],
          ),
          const SizedBox(height: 10),
          ClipRRect(
            borderRadius: BorderRadius.circular(999),
            child: LinearProgressIndicator(
              value: pct / 100,
              minHeight: 6,
              backgroundColor: c.navy10,
              valueColor: AlwaysStoppedAnimation<Color>(c.brandNavy900),
            ),
          ),
        ],
      ),
    );
  }
}

class _StepCard extends StatelessWidget {
  const _StepCard({
    required this.icon,
    required this.label,
    required this.subLabel,
    required this.status,
    this.onTap,
  });

  final IconData icon;
  final String label;
  final String subLabel;
  final String status;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final (fg, bg) = _iconColors(status, c);
    final enabled = onTap != null;

    return Opacity(
      opacity: enabled ? 1.0 : 0.55,
      child: GestureDetector(
        onTap: onTap,
        behavior: HitTestBehavior.opaque,
        child: Container(
          padding: const EdgeInsets.fromLTRB(14, 12, 14, 12),
          decoration: BoxDecoration(
            color: c.surface,
            borderRadius: BorderRadius.circular(14),
            border: Border.all(color: c.divider),
          ),
          child: Row(
            children: [
              Container(
                width: 40,
                height: 40,
                alignment: Alignment.center,
                decoration: BoxDecoration(
                  color: bg,
                  borderRadius: BorderRadius.circular(10),
                ),
                child: Icon(icon, size: 20, color: fg),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      label,
                      style: TextStyle(
                        fontSize: 14,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                        letterSpacing: -0.1,
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      subLabel,
                      style: TextStyle(fontSize: 12, color: c.textMuted),
                    ),
                  ],
                ),
              ),
              _StatusChip(status: status),
            ],
          ),
        ),
      ),
    );
  }

  static (Color, Color) _iconColors(String status, dynamic c) {
    return switch (status) {
      'approved' => (c.success, c.successSoft),
      'rejected' => (c.danger, c.dangerSoft),
      'submitted' => (c.warning, c.warningSoft),
      _ => (c.textMuted, c.navy10),
    };
  }
}

class _StatusChip extends StatelessWidget {
  const _StatusChip({required this.status});
  final String status;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final (label, fg, icon) = switch (status) {
      'approved' => ('Verified', c.success, Icons.check_circle_rounded),
      'submitted' => ('Pending', c.warning, Icons.schedule_rounded),
      'rejected' => ('Action needed', c.danger, Icons.error_rounded),
      _ => ('Not started', c.textMuted, Icons.radio_button_unchecked),
    };

    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(icon, size: 14, color: fg),
        const SizedBox(width: 4),
        Text(
          label,
          style: TextStyle(
            fontSize: 12,
            fontWeight: FontWeight.w700,
            color: fg,
          ),
        ),
      ],
    );
  }
}

class _CheckRow extends StatelessWidget {
  const _CheckRow({required this.label, required this.status});
  final String label;
  final String status;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final (fg, icon, trailing) = switch (status) {
      'approved' => (c.success, Icons.check_circle_rounded, 'Verified'),
      'submitted' => (c.warning, Icons.schedule_rounded, 'Under review'),
      'rejected' => (c.danger, Icons.error_rounded, 'Action required'),
      _ => (c.textMuted, Icons.radio_button_unchecked, 'Pending'),
    };

    return Row(
      children: [
        Icon(icon, size: 20, color: fg),
        const SizedBox(width: 12),
        Expanded(
          child: Text(
            label,
            style: TextStyle(
              fontSize: 14,
              fontWeight: FontWeight.w600,
              color: c.text,
            ),
          ),
        ),
        Text(
          trailing,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w600,
            color: fg,
          ),
        ),
      ],
    );
  }
}

class _ProgressDots extends StatelessWidget {
  const _ProgressDots({required this.count, required this.filled});
  final int count;
  final int filled;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: List.generate(count, (i) {
        final on = i < filled;
        return Padding(
          padding: const EdgeInsets.symmetric(horizontal: 4),
          child: AnimatedContainer(
            duration: const Duration(milliseconds: 220),
            width: on ? 26 : 8,
            height: 8,
            decoration: BoxDecoration(
              color: on ? c.info : c.navy20,
              borderRadius: BorderRadius.circular(999),
            ),
          ),
        );
      }),
    );
  }
}

class _BulletRow extends StatelessWidget {
  const _BulletRow({required this.text});
  final String text;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(top: 6),
          child: Container(
            width: 6,
            height: 6,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: c.danger,
            ),
          ),
        ),
        const SizedBox(width: 10),
        Expanded(
          child: Text(
            text,
            style: TextStyle(fontSize: 14, color: c.text, height: 1.4),
          ),
        ),
      ],
    );
  }
}

class _BottomCta extends StatelessWidget {
  const _BottomCta({
    required this.label,
    required this.onPressed,
    this.navy = false,
    this.destructive = false,
  });

  final String label;
  final VoidCallback? onPressed;
  final bool navy;
  final bool destructive;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    if (!navy && !destructive) {
      return Padding(
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
        child: SizedBox(
          height: 52,
          width: double.infinity,
          child: FilledButton(
            onPressed: onPressed,
            style: FilledButton.styleFrom(
              textStyle: const TextStyle(
                fontSize: 16,
                fontWeight: FontWeight.w700,
              ),
            ),
            child: Text(label),
          ),
        ),
      );
    }

    final bg = destructive ? c.danger : c.brandNavy900;

    return Padding(
      padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
      child: SizedBox(
        height: 52,
        width: double.infinity,
        child: FilledButton(
          onPressed: onPressed,
          style: FilledButton.styleFrom(
            backgroundColor: bg,
            foregroundColor: c.brandWhite,
            disabledBackgroundColor: bg.withValues(alpha: 0.35),
            disabledForegroundColor: c.brandWhite.withValues(alpha: 0.7),
            shape: const StadiumBorder(),
            textStyle: const TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w700,
            ),
          ),
          child: Text(label),
        ),
      ),
    );
  }
}

class _ErrorView extends StatelessWidget {
  const _ErrorView({required this.message, required this.onRetry});
  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Center(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.cloud_off_outlined, size: 48, color: c.textDisabled),
            const SizedBox(height: 16),
            Text(
              message,
              style: TextStyle(fontSize: 15, color: c.textMuted),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 20),
            OutlinedButton(onPressed: onRetry, child: const Text('Retry')),
          ],
        ),
      ),
    );
  }
}