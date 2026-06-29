import 'dart:async';

import 'package:flutter/material.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../../vehicle/state/vehicle_controller.dart';
import '../../verification/models/verification_models.dart';
import '../../verification/state/verification_controller.dart';
import '../state/provider_profile_controller.dart';
import 'emergency_contact_screen.dart';
import 'face_verification_screen.dart';
import 'guarantor_information_screen.dart';
import 'vehicle_information_screen.dart';
import 'verification_documents_screen.dart';

class VerificationIntroScreen extends StatefulWidget {
  const VerificationIntroScreen({
    super.key,
    required this.verificationController,
    this.vehicleController,
    this.profileController,
  });

  final VerificationController verificationController;
  final VehicleController? vehicleController;
  final ProviderProfileController? profileController;

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
    // Poll every 60 s so admin approve/reject updates without app restart.
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

  // ── Navigation helpers ────────────────────────────────────────────────────

  Future<void> _openDocuments() async {
    final status = widget.verificationController.latestStatus;
    final identityStatus = status?.steps
        .where((s) => s.step == 'identity')
        .firstOrNull
        ?.status ?? 'pending';

    final result = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => VerificationDocumentsScreen(
          verificationStatus: identityStatus,
          verificationController: widget.verificationController,
        ),
      ),
    );
    if (!mounted) return;
    if (result == true) {
      Navigator.of(context).pop(true);
    } else {
      // Came back without completing — refresh to reflect any partial submit.
      widget.verificationController.refreshStatus();
      setState(() {});
    }
  }

  Future<void> _openFace() async {
    final result = await Navigator.of(context).push<bool>(
      MaterialPageRoute(
        builder: (_) => FaceVerificationScreen(
          verificationController: widget.verificationController,
        ),
      ),
    );
    if (!mounted) return;
    if (result == true) {
      widget.verificationController.refreshStatus();
      Navigator.of(context).pop(true);
    } else {
      widget.verificationController.refreshStatus();
      setState(() {});
    }
  }

  Future<void> _openVehicle() async {
    final vc = widget.vehicleController;
    if (vc == null) return;
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => VehicleInformationScreen(vehicleController: vc),
      ),
    );
    if (!mounted) return;
    widget.verificationController.refreshStatus();
    setState(() {});
  }

  Future<void> _openGuarantor() async {
    final pc = widget.profileController;
    if (pc == null) return;
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => GuarantorInformationScreen(profileController: pc),
      ),
    );
    if (!mounted) return;
    widget.verificationController.refreshStatus();
    setState(() {});
  }

  Future<void> _openEmergency() async {
    final pc = widget.profileController;
    if (pc == null) return;
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => EmergencyContactScreen(profileController: pc),
      ),
    );
    if (!mounted) return;
    widget.verificationController.refreshStatus();
    setState(() {});
  }

  // ── Build ─────────────────────────────────────────────────────────────────

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: Colors.white,
      body: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            // ── Back arrow ────────────────────────────────────────
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 12, 20, 0),
              child: GestureDetector(
                behavior: HitTestBehavior.opaque,
                onTap: () => Navigator.of(context).pop(),
                child: const Align(
                  alignment: Alignment.centerLeft,
                  child: Icon(
                    Icons.arrow_back_ios_new,
                    size: 20,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
              ),
            ),
            const SizedBox(height: 16),
            // ── Title ─────────────────────────────────────────────
            const Padding(
              padding: EdgeInsets.symmetric(horizontal: 20),
              child: Text(
                'Verification & Documents',
                style: TextStyle(
                  fontSize: 20,
                  fontWeight: FontWeight.w800,
                  color: Color(0xFF1A1A1A),
                ),
              ),
            ),
            const SizedBox(height: 2),
            const Padding(
              padding: EdgeInsets.symmetric(horizontal: 20),
              child: Text(
                'Complete all steps to go online as a provider.',
                style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
              ),
            ),
            const SizedBox(height: 20),

            // ── Content ───────────────────────────────────────────
            if (_isLoading)
              const Expanded(
                child: Center(
                  child: CircularProgressIndicator(color: Color(0xFF4CAF50)),
                ),
              )
            else if (_loadError != null)
              Expanded(
                child: _ErrorView(message: _loadError!, onRetry: _load),
              )
            else
              ListenableBuilder(
                listenable: widget.verificationController,
                builder: (context, _) {
                  final status =
                      widget.verificationController.latestStatus;
                  if (status == null) {
                    return const Expanded(
                      child: Center(
                        child: CircularProgressIndicator(
                          color: Color(0xFF4CAF50),
                        ),
                      ),
                    );
                  }
                  return _StatusContent(
                    status: status,
                    onOpenDocuments: _openDocuments,
                    onOpenFace: _openFace,
                    onOpenVehicle: widget.vehicleController != null
                        ? _openVehicle
                        : null,
                    onOpenGuarantor: widget.profileController != null
                        ? _openGuarantor
                        : null,
                    onOpenEmergency: widget.profileController != null
                        ? _openEmergency
                        : null,
                  );
                },
              ),
          ],
        ),
      ),
    );
  }
}

// ── Status content ────────────────────────────────────────────────────────────

class _StatusContent extends StatelessWidget {
  const _StatusContent({
    required this.status,
    required this.onOpenDocuments,
    required this.onOpenFace,
    this.onOpenVehicle,
    this.onOpenGuarantor,
    this.onOpenEmergency,
  });

  final AllStatusResponse status;
  final VoidCallback onOpenDocuments;
  final VoidCallback onOpenFace;
  final VoidCallback? onOpenVehicle;
  final VoidCallback? onOpenGuarantor;
  final VoidCallback? onOpenEmergency;

  @override
  Widget build(BuildContext context) {
    final stepMap = {for (final s in status.steps) s.step: s};
    final action = _computeAction(stepMap);

    return Expanded(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // ── Overall status banner ───────────────────────────────
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 20),
            child: _OverallStatusBanner(overallStatus: status.overallStatus),
          ),
          const SizedBox(height: 20),

          // ── Step list ───────────────────────────────────────────
          Expanded(
            child: ListView(
              padding: const EdgeInsets.fromLTRB(20, 0, 20, 0),
              children: [
                _StepCard(
                  icon: Icons.badge_outlined,
                  label: 'Government ID',
                  step: stepMap['identity'],
                  canResubmit: true,
                  onResubmit: onOpenDocuments,
                ),
                const SizedBox(height: 10),
                _StepCard(
                  icon: Icons.drive_eta_outlined,
                  label: "Driver's License",
                  step: stepMap['licence'],
                  canResubmit: true,
                  onResubmit: onOpenDocuments,
                ),
                _LicenceExpiryBanner(licenceStep: stepMap['licence']),
                const SizedBox(height: 10),
                _StepCard(
                  icon: Icons.face_outlined,
                  label: 'Face Verification',
                  step: stepMap['face'],
                  canResubmit: true,
                  onResubmit: onOpenFace,
                ),
                const SizedBox(height: 10),
                _StepCard(
                  icon: Icons.two_wheeler_outlined,
                  label: 'Vehicle',
                  step: stepMap['vehicle'],
                  canResubmit: true,
                  showActionWhenPending: true,
                  actionLabel: 'Go to Vehicle',
                  onResubmit: onOpenVehicle,
                ),
                const SizedBox(height: 10),
                _StepCard(
                  icon: Icons.people_outline,
                  label: 'Guarantor',
                  step: stepMap['guarantor'],
                  canResubmit: true,
                  showActionWhenPending: true,
                  actionLabel: 'Add Guarantor',
                  onResubmit: onOpenGuarantor,
                ),
                const SizedBox(height: 10),
                _StepCard(
                  icon: Icons.emergency_outlined,
                  label: 'Emergency Contact',
                  step: stepMap['emergency'],
                  canResubmit: true,
                  showActionWhenPending: true,
                  actionLabel: 'Add Emergency Contact',
                  onResubmit: onOpenEmergency,
                ),
                const SizedBox(height: 20),
              ],
            ),
          ),

          // ── Bottom CTA ──────────────────────────────────────────
          Padding(
            padding: const EdgeInsets.fromLTRB(20, 0, 20, 16),
            child: _BottomCta(
              action: action,
              stepMap: stepMap,
              onOpenDocuments: onOpenDocuments,
              onOpenFace: onOpenFace,
              onOpenVehicle: onOpenVehicle,
              onOpenGuarantor: onOpenGuarantor,
              onOpenEmergency: onOpenEmergency,
            ),
          ),
        ],
      ),
    );
  }
}

// ── Action enum ───────────────────────────────────────────────────────────────

enum _CtaAction {
  startDocuments,
  continueDocuments,
  resubmitDocuments,
  startFace,
  resubmitFace,
  goToVehicle,
  goToGuarantor,
  goToEmergency,
  awaitingReview,
}

_CtaAction _computeAction(Map<String, VerificationStepSummary?> m) {
  final identity = m['identity'];
  final licence = m['licence'];
  final face = m['face'];

  final identityStatus = identity?.status ?? 'pending';
  final licenceStatus = licence?.status ?? 'pending';
  final faceStatus = face?.status ?? 'pending';

  if (identityStatus == 'rejected' || licenceStatus == 'rejected') {
    return _CtaAction.resubmitDocuments;
  }
  if (faceStatus == 'rejected') {
    return _CtaAction.resubmitFace;
  }
  if (identityStatus == 'pending' || licenceStatus == 'pending') {
    final anyProgress = identityStatus != 'pending' ||
        licenceStatus != 'pending' ||
        faceStatus != 'pending';
    return anyProgress
        ? _CtaAction.continueDocuments
        : _CtaAction.startDocuments;
  }
  if (faceStatus == 'pending') {
    return _CtaAction.startFace;
  }

  // All three uploadable steps (identity, licence, face) are submitted or
  // approved. Now check whether vehicle, guarantor, and emergency also need
  // attention before the provider can reach awaitingReview.
  final vehicleStatus = m['vehicle']?.status ?? 'pending';
  if (vehicleStatus == 'pending' || vehicleStatus == 'rejected') {
    return _CtaAction.goToVehicle;
  }

  final guarantorStatus = m['guarantor']?.status ?? 'pending';
  if (guarantorStatus == 'pending' || guarantorStatus == 'rejected') {
    return _CtaAction.goToGuarantor;
  }

  final emergencyStatus = m['emergency']?.status ?? 'pending';
  if (emergencyStatus == 'pending' || emergencyStatus == 'rejected') {
    return _CtaAction.goToEmergency;
  }

  return _CtaAction.awaitingReview;
}

// ── Bottom CTA ────────────────────────────────────────────────────────────────

class _BottomCta extends StatelessWidget {
  const _BottomCta({
    required this.action,
    required this.stepMap,
    required this.onOpenDocuments,
    required this.onOpenFace,
    this.onOpenVehicle,
    this.onOpenGuarantor,
    this.onOpenEmergency,
  });

  final _CtaAction action;
  final Map<String, VerificationStepSummary?> stepMap;
  final VoidCallback onOpenDocuments;
  final VoidCallback onOpenFace;
  final VoidCallback? onOpenVehicle;
  final VoidCallback? onOpenGuarantor;
  final VoidCallback? onOpenEmergency;

  @override
  Widget build(BuildContext context) {
    final (label, onPressed) = switch (action) {
      _CtaAction.startDocuments => (
        'Start Verification',
        onOpenDocuments,
      ),
      _CtaAction.continueDocuments => (
        'Continue Verification',
        onOpenDocuments,
      ),
      _CtaAction.resubmitDocuments => (
        'Re-submit Documents',
        onOpenDocuments,
      ),
      _CtaAction.startFace => ('Start Face Verification', onOpenFace),
      _CtaAction.resubmitFace => ('Re-submit Face Verification', onOpenFace),
      _CtaAction.goToVehicle => (
        'Complete Vehicle Registration',
        onOpenVehicle,
      ),
      _CtaAction.goToGuarantor => (
        'Add Guarantor Information',
        onOpenGuarantor,
      ),
      _CtaAction.goToEmergency => (
        'Add Emergency Contact',
        onOpenEmergency,
      ),
      _CtaAction.awaitingReview => ('Submitted — Awaiting Review', null),
    };

    return SizedBox(
      height: 52,
      width: double.infinity,
      child: FilledButton(
        onPressed: onPressed,
        style: FilledButton.styleFrom(
          backgroundColor: const Color(0xFF4CAF50),
          disabledBackgroundColor: const Color(0xFF4CAF50).withValues(alpha: 0.35),
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(999),
          ),
        ),
        child: Text(
          label,
          style: const TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w700,
            color: Colors.white,
          ),
        ),
      ),
    );
  }
}

// ── Overall status banner ─────────────────────────────────────────────────────

class _OverallStatusBanner extends StatelessWidget {
  const _OverallStatusBanner({required this.overallStatus});

  final String overallStatus;

  @override
  Widget build(BuildContext context) {
    final (label, bg, fg, icon) = switch (overallStatus) {
      'verified' => (
        'All steps verified',
        const Color(0xFFE8F5E9),
        const Color(0xFF2E7D32),
        Icons.verified_outlined,
      ),
      'pending_review' => (
        'Submitted — Pending Admin Review',
        const Color(0xFFFFF8E1),
        const Color(0xFFF57F17),
        Icons.hourglass_top_outlined,
      ),
      'in_progress' => (
        'In Progress — Some steps submitted',
        const Color(0xFFE3F2FD),
        const Color(0xFF1565C0),
        Icons.upload_outlined,
      ),
      'rejected' => (
        'Action Required — One or more steps rejected',
        const Color(0xFFFFEBEE),
        const Color(0xFFC62828),
        Icons.error_outline,
      ),
      'suspended' => (
        'Account Suspended — Contact support',
        const Color(0xFFECEFF1),
        const Color(0xFF37474F),
        Icons.block_outlined,
      ),
      _ => (
        'Not yet verified — Submit your documents',
        const Color(0xFFFFF3E0),
        const Color(0xFFE65100),
        Icons.info_outline,
      ),
    };

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        children: [
          Icon(icon, size: 18, color: fg),
          const SizedBox(width: 10),
          Expanded(
            child: Text(
              label,
              style: TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w700,
                color: fg,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

// ── Step card ─────────────────────────────────────────────────────────────────

class _StepCard extends StatelessWidget {
  const _StepCard({
    required this.icon,
    required this.label,
    required this.step,
    required this.canResubmit,
    this.showActionWhenPending = false,
    this.actionLabel,
    this.onResubmit,
  });

  final IconData icon;
  final String label;
  final VerificationStepSummary? step;
  final bool canResubmit;
  final bool showActionWhenPending;
  final String? actionLabel;
  final VoidCallback? onResubmit;

  @override
  Widget build(BuildContext context) {
    final status = step?.status ?? 'pending';
    final reason = step?.rejectionReason;
    final isRejected = status == 'rejected';

    final (statusLabel, statusColor) = _statusDisplay(status);

    return Container(
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        color: isRejected ? const Color(0xFFFFF5F5) : const Color(0xFFF9F9F9),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(
          color: isRejected
              ? const Color(0xFFFFCDD2)
              : const Color(0xFFEEEEEE),
        ),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(icon, size: 20, color: const Color(0xFF555555)),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  label,
                  style: const TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w600,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
              ),
              _StatusChip(label: statusLabel, color: statusColor),
            ],
          ),
          if (isRejected && reason != null && reason.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(
              'Reason: $reason',
              style: const TextStyle(
                fontSize: 12,
                color: Color(0xFFC62828),
                height: 1.4,
              ),
            ),
          ],
          if (canResubmit && onResubmit != null &&
              (isRejected ||
                  (showActionWhenPending && status == 'pending'))) ...[
            const SizedBox(height: 10),
            Align(
              alignment: Alignment.centerRight,
              child: GestureDetector(
                onTap: onResubmit,
                child: Container(
                  padding: const EdgeInsets.symmetric(
                    horizontal: 14,
                    vertical: 6,
                  ),
                  decoration: BoxDecoration(
                    color: isRejected
                        ? const Color(0xFFC62828)
                        : const Color(0xFF4CAF50),
                    borderRadius: BorderRadius.circular(999),
                  ),
                  child: Text(
                    actionLabel ?? (isRejected ? 'Re-upload' : 'Complete'),
                    style: const TextStyle(
                      fontSize: 12,
                      fontWeight: FontWeight.w700,
                      color: Colors.white,
                    ),
                  ),
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }
}

(String, Color) _statusDisplay(String status) => switch (status) {
  'approved' => ('Approved', const Color(0xFF2E7D32)),
  'submitted' => ('Pending Review', const Color(0xFFF57F17)),
  'rejected' => ('Rejected', const Color(0xFFC62828)),
  _ => ('Not submitted', const Color(0xFF888888)),
};

// ── Status chip ───────────────────────────────────────────────────────────────

class _StatusChip extends StatelessWidget {
  const _StatusChip({required this.label, required this.color});

  final String label;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: color.withValues(alpha: 0.12),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        label,
        style: TextStyle(
          fontSize: 11,
          fontWeight: FontWeight.w700,
          color: color,
        ),
      ),
    );
  }
}

// ── Licence expiry banner ─────────────────────────────────────────────────────

class _LicenceExpiryBanner extends StatelessWidget {
  const _LicenceExpiryBanner({required this.licenceStep});

  final VerificationStepSummary? licenceStep;

  @override
  Widget build(BuildContext context) {
    final expiry = licenceStep?.licenceExpiryDate;
    if (expiry == null) return const SizedBox.shrink();

    final now = DateTime.now();
    final daysUntilExpiry = expiry.difference(now).inDays;

    // Outside warning window — no banner.
    if (daysUntilExpiry > 30) return const SizedBox.shrink();

    final String message;
    final Color bg;
    final Color fg;
    final IconData icon;

    if (daysUntilExpiry < -30) {
      // Grace period already over — gate will block. Backend handles enforcement.
      return const SizedBox.shrink();
    } else if (daysUntilExpiry < 0) {
      // Expired but within 30-day grace period.
      final daysOverdue = daysUntilExpiry.abs();
      message =
          'Your licence expired $daysOverdue day${daysOverdue == 1 ? '' : 's'} ago. '
          'You have ${30 - daysOverdue} day${(30 - daysOverdue) == 1 ? '' : 's'} left to renew before you are blocked from going online.';
      bg = const Color(0xFFFFEBEE);
      fg = const Color(0xFFC62828);
      icon = Icons.warning_amber_outlined;
    } else {
      // Expiring within 30 days.
      final expiryLabel =
          '${expiry.day}/${expiry.month}/${expiry.year}';
      message = daysUntilExpiry == 0
          ? 'Your licence expires today. Renew it to avoid losing access.'
          : 'Your licence expires in $daysUntilExpiry day${daysUntilExpiry == 1 ? '' : 's'} ($expiryLabel). Renew it soon.';
      bg = const Color(0xFFFFF8E1);
      fg = const Color(0xFFF57F17);
      icon = Icons.access_time_outlined;
    }

    return Padding(
      padding: const EdgeInsets.only(top: 6),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        decoration: BoxDecoration(
          color: bg,
          borderRadius: BorderRadius.circular(10),
        ),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Icon(icon, size: 16, color: fg),
            const SizedBox(width: 8),
            Expanded(
              child: Text(
                message,
                style: TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w500,
                  color: fg,
                  height: 1.4,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ── Error view ────────────────────────────────────────────────────────────────

class _ErrorView extends StatelessWidget {
  const _ErrorView({required this.message, required this.onRetry});

  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(
              Icons.cloud_off_outlined,
              size: 48,
              color: Color(0xFFCCCCCC),
            ),
            const SizedBox(height: 16),
            Text(
              message,
              style: const TextStyle(fontSize: 14, color: Color(0xFF888888)),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 20),
            OutlinedButton(
              onPressed: onRetry,
              child: const Text('Retry'),
            ),
          ],
        ),
      ),
    );
  }
}
