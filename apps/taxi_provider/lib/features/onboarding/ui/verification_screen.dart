import 'package:flutter/material.dart';

import '../../../core/theme/karryam_theme.dart';
import '../../../core/widgets/karryam_ui.dart';
import '../../auth/models/provider_auth_models.dart';
import '../../auth/state/provider_auth_controller.dart';

/// Where the driver's account review stands, and what is still needed
/// before they can go online.
class VerificationScreen extends StatefulWidget {
  const VerificationScreen({
    super.key,
    required this.auth,
    this.onUpdateDetails,
  });
  final ProviderAuthController auth;
  final VoidCallback? onUpdateDetails;

  @override
  State<VerificationScreen> createState() => _VerificationScreenState();
}

class _VerificationScreenState extends State<VerificationScreen> {
  @override
  void initState() {
    super.initState();
    widget.auth.refreshProfile();
  }

  static bool _passed(String status) => const [
    'verified',
    'approved',
    'compliant',
    'active',
    'operational',
  ].contains(status);

  static bool _waiting(String status) => const [
    'pending',
    'pending_review',
    'submitted',
    'in_review',
    'under_review',
  ].contains(status);

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.auth,
      builder: (context, _) {
        final p = widget.auth.state.provider;
        if (p == null) return const SizedBox.shrink();
        final checks = <(String, String, String)>[
          (
            'Identity',
            p.identityVerificationStatus,
            'Your ID and licence are checked against your details.',
          ),
          (
            'Documents',
            p.documentComplianceStatus,
            'Licence, ID and vehicle papers are valid and in date.',
          ),
          (
            'Vehicle',
            p.vehicleVerificationStatus,
            'Your car is inspected and approved.',
          ),
          (
            'Vehicle status',
            p.vehicleOperationalStatus,
            'Your car is cleared to take trips.',
          ),
        ];
        final allPassed = checks.every((c) => _passed(c.$2));
        return KarryamPage(
          title: 'Verification',
          subtitle: allPassed
              ? 'You are cleared to drive'
              : 'We review every new driver',
          body: RefreshIndicator(
            onRefresh: widget.auth.refreshProfile,
            child: ListView(
              padding: const EdgeInsets.all(20),
              children: [
                KarryamPanel(
                  child: Text(
                    allPassed
                        ? 'All checks passed. Go online from the home screen to start receiving trips.'
                        : 'Our team checks your documents before you can go online. '
                              'This usually takes one to two working days. Pull down to refresh.',
                  ),
                ),
                const SizedBox(height: 16),
                for (final (label, status, help) in checks)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 10),
                    child: KarryamPanel(
                      child: Row(
                        children: [
                          Expanded(
                            child: Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text(
                                  label,
                                  style: const TextStyle(
                                    fontWeight: FontWeight.w600,
                                  ),
                                ),
                                const SizedBox(height: 2),
                                Text(
                                  help,
                                  style: TextStyle(
                                    color: KarryamColors.of(context).textMuted,
                                    fontSize: 12,
                                  ),
                                ),
                              ],
                            ),
                          ),
                          const SizedBox(width: 8),
                          KarryamChip(
                            humanize(status),
                            tone: _passed(status)
                                ? KarryamTone.good
                                : _waiting(status)
                                ? KarryamTone.warning
                                : status.contains('reject') ||
                                      status.contains('fail') ||
                                      status.contains('suspend') ||
                                      status == 'expired'
                                ? KarryamTone.bad
                                : KarryamTone.neutral,
                          ),
                        ],
                      ),
                    ),
                  ),
                if (!p.isActive)
                  Padding(
                    padding: const EdgeInsets.only(top: 6),
                    child: KarryamChip(
                      'Account ${humanize(p.accountStatus).toLowerCase()}',
                      tone: KarryamTone.bad,
                    ),
                  ),
                if (widget.onUpdateDetails != null) ...[
                  const SizedBox(height: 16),
                  OutlinedButton(
                    onPressed: widget.onUpdateDetails,
                    child: const Text('Update details or documents'),
                  ),
                ],
              ],
            ),
          ),
        );
      },
    );
  }
}

/// Whether [p] is fully cleared, for badges elsewhere.
bool driverCleared(TaxiProvider p) => [
  p.identityVerificationStatus,
  p.documentComplianceStatus,
  p.vehicleVerificationStatus,
].every(_VerificationScreenState._passed);
