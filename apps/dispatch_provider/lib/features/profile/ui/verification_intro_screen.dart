import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/async_content.dart';
import '../../../shared/widgets/journey_icon.dart';
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
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _load();
    });
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
    final result = await widget.verificationController.loadVerificationStatus();
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
      MaterialPageRoute(builder: (_) => BikeListScreen(vehicleController: vc)),
    );
    if (!mounted) return;
    widget.verificationController.refreshStatus();
    setState(() {});
  }

  void _openHelp() {
    showModalBottomSheet<void>(
      context: context,
      useSafeArea: true,
      showDragHandle: true,
      isScrollControlled: true,
      barrierColor: Colors.black.withValues(alpha: .5),
      builder: (sheet) => const Padding(
        padding: EdgeInsets.fromLTRB(24, 0, 24, 32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            AccountEmblem(symbol: JourneySymbol.contact, size: 56),
            SizedBox(height: 20),
            Text(
              'Verification help',
              style: TextStyle(fontSize: 24, fontWeight: FontWeight.w700),
            ),
            SizedBox(height: 12),
            Text(
              'Check the message beside each document. If you need help with a review decision, open Help and support in Account settings.',
            ),
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) => ListenableBuilder(
    listenable: widget.verificationController,
    builder: (context, _) {
      final status = widget.verificationController.latestStatus;
      final overall = status?.overallStatus ?? 'not_started';
      final c = context.palette;
      final color = overall == 'verified'
          ? c.success
          : ['rejected', 'suspended'].contains(overall)
          ? c.danger
          : overall == 'pending_review'
          ? c.warning
          : c.brandTeal600;
      final identity = status?.steps
          .where((s) => s.step == 'identity')
          .firstOrNull;
      final identityReady = [
        'submitted',
        'approved',
      ].contains(identity?.status);
      return AccountPage(
        title: 'Documents',
        symbol: JourneySymbol.review,
        onRefresh: _load,
        description: 'Your verification progress and document review status.',
        actions: [
          IconButton(
            tooltip: 'Verification help',
            onPressed: _openHelp,
            icon: const Icon(Icons.help_outline_rounded),
          ),
        ],
        children: [
          if (_isLoading)
            const SizedBox(height: 220, child: ContentSkeleton(rows: 2))
          else if (_loadError != null)
            ActionNotice(message: _loadError!, isError: true, onRetry: _load)
          else if (status != null) ...[
            DeliveryPanel(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Row(
                    children: [
                      AccountEmblem(symbol: JourneySymbol.shield, color: color),
                      const SizedBox(width: 14),
                      Expanded(
                        child: Text(
                          switch (overall) {
                            'verified' => 'Identity verified',
                            'pending_review' => 'Under review',
                            'rejected' => 'Changes needed',
                            'suspended' => 'Account restricted',
                            'not_started' => 'Ready to get started',
                            _ => 'Verification in progress',
                          },
                          style: Theme.of(
                            context,
                          ).textTheme.titleMedium?.copyWith(color: color),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 18),
                  ClipRRect(
                    borderRadius: BorderRadius.circular(8),
                    child: LinearProgressIndicator(
                      value: status.completionPercentage.clamp(0, 100) / 100,
                      minHeight: 6,
                      color: color,
                      backgroundColor: c.divider,
                    ),
                  ),
                  const SizedBox(height: 12),
                  Text(
                    '${status.completionPercentage.clamp(0, 100)}% complete',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                  if (overall == 'pending_review') ...[
                    const SizedBox(height: 12),
                    const Text(
                      'Your submitted details are being reviewed. Pull down to refresh the status.',
                    ),
                  ],
                  if (overall == 'suspended') ...[
                    const SizedBox(height: 12),
                    const Text(
                      'Contact support for help with your account restriction.',
                    ),
                  ],
                ],
              ),
            ),
            const SizedBox(height: 24),
            for (final step in status.steps)
              Padding(
                padding: const EdgeInsets.only(bottom: 14),
                child: _document(
                  step,
                  overall: overall,
                  identityReady: identityReady,
                ),
              ),
            if (widget.vehicleController != null) ...[
              const SizedBox(height: 10),
              DeliveryPanel(
                padding: EdgeInsets.zero,
                child: AccountMenuTile(
                  title: 'Vehicles',
                  subtitle: 'View vehicle registration and review status',
                  symbol: JourneySymbol.fleet,
                  onTap: _openVehicle,
                ),
              ),
            ],
          ],
        ],
      );
    },
  );

  Widget _document(
    VerificationStepSummary step, {
    required String overall,
    required bool identityReady,
  }) {
    final c = context.palette;
    final approved = step.status == 'approved';
    final submitted = step.status == 'submitted';
    final rejected = step.status == 'rejected';
    final color = approved
        ? c.success
        : rejected
        ? c.danger
        : submitted
        ? c.warning
        : c.textMuted;
    final (label, symbol) = switch (step.step) {
      'identity' => ('Identity', JourneySymbol.contact),
      'face' => ('Selfie', JourneySymbol.profile),
      'licence' => ("Driver's licence", JourneySymbol.review),
      'vehicle' => ('Vehicle', JourneySymbol.fleet),
      'guarantor' => ('Guarantor', JourneySymbol.partner),
      'emergency_contact' => ('Emergency contact', JourneySymbol.contact),
      _ => (step.step.replaceAll('_', ' '), JourneySymbol.review),
    };
    VoidCallback? open;
    if (!approved &&
        !submitted &&
        !['suspended', 'verified', 'pending_review'].contains(overall)) {
      open = switch (step.step) {
        'identity' => _openIdentity,
        'face' => identityReady ? _openFace : null,
        'licence' => rejected ? _openLicenceResubmit : _openLicence,
        'vehicle' => widget.vehicleController == null ? null : _openVehicle,
        _ => null,
      };
    }
    return DeliveryPanel(
      onTap: open,
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Row(
            children: [
              AccountEmblem(symbol: symbol, color: color),
              const SizedBox(width: 14),
              Expanded(
                child: Text(
                  label,
                  style: Theme.of(context).textTheme.titleSmall,
                ),
              ),
              if (approved)
                Icon(Icons.check_rounded, color: c.success)
              else if (open != null)
                const Icon(Icons.chevron_right_rounded),
            ],
          ),
          const SizedBox(height: 14),
          Text(
            approved
                ? 'Verified'
                : submitted
                ? 'Under review'
                : rejected
                ? 'Changes needed'
                : step.status == 'pending'
                ? 'Not started'
                : step.status.replaceAll('_', ' '),
            style: Theme.of(context).textTheme.bodyMedium?.copyWith(
              color: color,
              fontWeight: FontWeight.w700,
            ),
          ),
          if (step.isOptional) ...[
            const SizedBox(height: 5),
            const Text('Optional'),
          ],
          if (step.step == 'face' &&
              !identityReady &&
              !approved &&
              !submitted) ...[
            const SizedBox(height: 6),
            const Text('Submit your identity details first.'),
          ],
          if (step.rejectionReason?.isNotEmpty == true) ...[
            const SizedBox(height: 8),
            Text(step.rejectionReason!),
          ],
          if (open != null) ...[
            const SizedBox(height: 12),
            Align(
              alignment: Alignment.centerLeft,
              child: TextButton(
                onPressed: open,
                child: Text(rejected ? 'Update document' : 'Continue'),
              ),
            ),
          ],
        ],
      ),
    );
  }
}
