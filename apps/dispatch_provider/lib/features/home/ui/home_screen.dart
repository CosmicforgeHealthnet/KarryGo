import 'package:flutter/material.dart';
import '../../verification/state/verification_controller.dart';

class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key, required this.verificationController});

  final VerificationController verificationController;

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  @override
  void initState() {
    super.initState();
    // Load verification status when the dashboard home tab opens.
    widget.verificationController.loadVerificationStatus();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.verificationController,
      builder: (context, _) {
        final ctrl = widget.verificationController;
        return Scaffold(
          backgroundColor: const Color(0xFFF5F5F5),
          body: SafeArea(
            child: RefreshIndicator(
              color: const Color(0xFF4CAF50),
              onRefresh: () =>
                  widget.verificationController.loadVerificationStatus(),
              child: ListView(
                padding: EdgeInsets.fromLTRB(
                  24,
                  24,
                  24,
                  // Clearance for the floating bottom nav (70px + 12px margin)
                  // plus the system navigation bar inset.
                  140 + MediaQuery.of(context).padding.bottom,
                ),
                children: [
                  const Text(
                    'Home',
                    style: TextStyle(
                      fontSize: 22,
                      fontWeight: FontWeight.w800,
                      color: Color(0xFF1A1A1A),
                      letterSpacing: -0.3,
                    ),
                  ),
                  const SizedBox(height: 20),
                  if (ctrl.isLoading && ctrl.latestStatus == null)
                    const Center(
                      child: Padding(
                        padding: EdgeInsets.symmetric(vertical: 32),
                        child: CircularProgressIndicator(
                          color: Color(0xFF4CAF50),
                          strokeWidth: 2.5,
                        ),
                      ),
                    )
                  else if (ctrl.latestStatus != null)
                    _VerificationCard(
                      overallStatus: ctrl.latestStatus!.overallStatus,
                      completionPercentage:
                          ctrl.latestStatus!.completionPercentage,
                    ),
                ],
              ),
            ),
          ),
        );
      },
    );
  }
}

// ── Verification status card ──────────────────────────────────────────────────

class _VerificationCard extends StatelessWidget {
  const _VerificationCard({
    required this.overallStatus,
    required this.completionPercentage,
  });

  final String overallStatus;
  final int completionPercentage;

  @override
  Widget build(BuildContext context) {
    final cfg = _statusConfig(overallStatus);
    return Container(
      padding: const EdgeInsets.all(18),
      decoration: BoxDecoration(
        color: cfg.bgColor,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: cfg.borderColor, width: 1),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(cfg.icon, color: cfg.iconColor, size: 24),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                  cfg.title,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                    color: cfg.titleColor,
                  ),
                ),
              ),
              if (overallStatus == 'verified')
                const Icon(
                  Icons.verified_rounded,
                  color: Color(0xFF4CAF50),
                  size: 22,
                ),
            ],
          ),
          const SizedBox(height: 8),
          Text(
            cfg.message,
            style: const TextStyle(
              fontSize: 13,
              color: Color(0xFF555555),
              height: 1.45,
            ),
          ),
          if (overallStatus != 'verified' && completionPercentage > 0) ...[
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: ClipRRect(
                    borderRadius: BorderRadius.circular(4),
                    child: LinearProgressIndicator(
                      value: completionPercentage / 100,
                      minHeight: 6,
                      backgroundColor: const Color(0xFFE0E0E0),
                      color: cfg.iconColor,
                    ),
                  ),
                ),
                const SizedBox(width: 10),
                Text(
                  '$completionPercentage%',
                  style: TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.w600,
                    color: cfg.iconColor,
                  ),
                ),
              ],
            ),
          ],
        ],
      ),
    );
  }

  static _StatusConfig _statusConfig(String status) {
    switch (status) {
      case 'verified':
        return const _StatusConfig(
          bgColor: Color(0xFFE8F5E9),
          borderColor: Color(0xFFA5D6A7),
          icon: Icons.check_circle_outline_rounded,
          iconColor: Color(0xFF2E7D32),
          titleColor: Color(0xFF1B5E20),
          title: 'Verification Complete',
          message:
              'Your account is verified. You can now start accepting orders.',
        );
      case 'pending_review':
        return const _StatusConfig(
          bgColor: Color(0xFFFFF8E1),
          borderColor: Color(0xFFFFE082),
          icon: Icons.hourglass_top_rounded,
          iconColor: Color(0xFFF9A825),
          titleColor: Color(0xFF1A1A1A),
          title: 'Under Review',
          message:
              'Your documents have been submitted and are currently under review. '
              "We'll notify you once it's complete.",
        );
      case 'in_progress':
        return const _StatusConfig(
          bgColor: Color(0xFFE3F2FD),
          borderColor: Color(0xFF90CAF9),
          icon: Icons.upload_file_rounded,
          iconColor: Color(0xFF1565C0),
          titleColor: Color(0xFF1A1A1A),
          title: 'Verification In Progress',
          message:
              'Some documents have been submitted and are being reviewed. '
              'Please ensure all steps are completed.',
        );
      case 'rejected':
        return const _StatusConfig(
          bgColor: Color(0xFFFFEBEE),
          borderColor: Color(0xFFEF9A9A),
          icon: Icons.cancel_outlined,
          iconColor: Color(0xFFC62828),
          titleColor: Color(0xFF1A1A1A),
          title: 'Verification Rejected',
          message:
              'One or more documents were rejected. Please resubmit the '
              'required documents to continue.',
        );
      case 'suspended':
        return const _StatusConfig(
          bgColor: Color(0xFFFFF3E0),
          borderColor: Color(0xFFFFCC80),
          icon: Icons.warning_amber_rounded,
          iconColor: Color(0xFFE65100),
          titleColor: Color(0xFF1A1A1A),
          title: 'Account Suspended',
          message:
              'Your account has been suspended. Please contact support for '
              'more information.',
        );
      default: // not_started or unknown
        return const _StatusConfig(
          bgColor: Color(0xFFF3F3F3),
          borderColor: Color(0xFFDDDDDD),
          icon: Icons.person_outline_rounded,
          iconColor: Color(0xFF555555),
          titleColor: Color(0xFF1A1A1A),
          title: 'Complete Your Verification',
          message:
              'Your identity has not been verified yet. Please complete your '
              'profile setup to start accepting orders.',
        );
    }
  }
}

class _StatusConfig {
  const _StatusConfig({
    required this.bgColor,
    required this.borderColor,
    required this.icon,
    required this.iconColor,
    required this.titleColor,
    required this.title,
    required this.message,
  });

  final Color bgColor;
  final Color borderColor;
  final IconData icon;
  final Color iconColor;
  final Color titleColor;
  final String title;
  final String message;
}
