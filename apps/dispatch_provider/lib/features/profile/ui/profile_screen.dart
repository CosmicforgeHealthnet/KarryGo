import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import '../models/provider_profile_models.dart';
import '../state/provider_profile_controller.dart';
import '../../verification/state/verification_controller.dart';
import '../../vehicle/state/vehicle_controller.dart';
import 'widgets/profile_menu_item.dart';
import 'profile_info_screen.dart';
import 'verification_intro_screen.dart';
import 'vehicle_information_screen.dart';
import 'safety_emergency_screen.dart';

class ProfileScreen extends StatefulWidget {
  final ProviderProfileController profileController;
  final VerificationController verificationController;
  final VehicleController vehicleController;
  final VoidCallback? onLogout;
  final VoidCallback? onAccountDeleted;

  const ProfileScreen({
    super.key,
    required this.profileController,
    required this.verificationController,
    required this.vehicleController,
    this.onLogout,
    this.onAccountDeleted,
  });

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  bool _isLoading = false;
  bool _isLoggingOut = false;
  bool _isDeletingAccount = false;

  @override
  void initState() {
    super.initState();
    _fetchProfile();
  }

  Future<void> _fetchProfile() async {
    setState(() => _isLoading = true);
    await Future.wait([
      widget.profileController.loadMe(),
      widget.profileController.loadStats(),
      widget.verificationController.loadVerificationStatus(),
    ]);
    if (mounted) {
      debugPrint(
        'photoUrl: ${widget.profileController.profile?.profilePhotoUrl}',
      );
      setState(() => _isLoading = false);
    }
  }

  void _confirmLogout() {
    showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        title: const Text(
          'Log Out',
          style: TextStyle(fontWeight: FontWeight.w800),
        ),
        content: const Text('Are you sure you want to log out?'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            style: TextButton.styleFrom(
              foregroundColor: const Color(0xFFE53935),
            ),
            child: const Text(
              'Log Out',
              style: TextStyle(fontWeight: FontWeight.w700),
            ),
          ),
        ],
      ),
    ).then((confirmed) {
      if (confirmed == true && mounted) {
        _doLogout();
      }
    });
  }

  Future<void> _doLogout() async {
    setState(() => _isLoggingOut = true);
    try {
      widget.onLogout?.call();
    } finally {
      if (mounted) setState(() => _isLoggingOut = false);
    }
  }

  void _confirmDeleteAccount() {
    showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        title: const Text(
          'Delete Account',
          style: TextStyle(fontWeight: FontWeight.w800),
        ),
        content: const Text(
          'This will permanently delete your account and you will lose all your data. This action cannot be undone.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('Cancel'),
          ),
          TextButton(
            onPressed: () => Navigator.pop(ctx, true),
            style: TextButton.styleFrom(
              foregroundColor: const Color(0xFFE53935),
            ),
            child: const Text(
              'Delete Account',
              style: TextStyle(fontWeight: FontWeight.w700),
            ),
          ),
        ],
      ),
    ).then((confirmed) {
      if (confirmed == true && mounted) {
        _doDeleteAccount();
      }
    });
  }

  Future<void> _doDeleteAccount() async {
    setState(() => _isDeletingAccount = true);
    final result = await widget.profileController.deleteAccount();
    if (!mounted) return;
    result.when(
      success: (_) {
        widget.onAccountDeleted?.call();
      },
      failure: (error) {
        setState(() => _isDeletingAccount = false);
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(error.message),
            backgroundColor: const Color(0xFFE53935),
          ),
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    final profile = widget.profileController.profile;

    return Scaffold(
      backgroundColor: const Color(0xFFF7F7F7),
      body: SafeArea(
        child: _isLoading
            ? const Center(
                child: CircularProgressIndicator(color: Color(0xFF4CAF50)),
              )
            : profile == null
            ? Center(
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    const Text(
                      'Failed to load profile details.',
                      style: TextStyle(color: Color(0xFF888888)),
                    ),
                    const SizedBox(height: 12),
                    OutlinedButton(
                      onPressed: _fetchProfile,
                      child: const Text('Retry'),
                    ),
                  ],
                ),
              )
            : ListView(
                padding: const EdgeInsets.fromLTRB(
                  20,
                  16,
                  20,
                  // Extra bottom clearance for the floating bottom nav.
                  24,
                ),
                children: [
                  const Text(
                    'Profile',
                    style: TextStyle(
                      fontSize: 22,
                      fontWeight: FontWeight.w800,
                      color: Color(0xFF1A1A1A),
                    ),
                  ),
                  const SizedBox(height: 2),
                  const Text(
                    'Manage your Personal Information and Account',
                    style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
                  ),
                  const SizedBox(height: 24),

                  // ── Header: avatar, name, stats, badges ─────────
                  Center(
                    child: Column(
                      children: [
                        _ProfileAvatar(
                          photoUrl: profile.profilePhotoUrl,
                          fallbackInitial:
                              (profile.fullName?.isNotEmpty == true)
                              ? profile.fullName![0].toUpperCase()
                              : '?',
                        ),
                        const SizedBox(height: 12),
                        Text(
                          profile.fullName ?? 'Unnamed Provider',
                          style: const TextStyle(
                            fontSize: 20,
                            fontWeight: FontWeight.w800,
                            color: Color(0xFF1A1A1A),
                          ),
                          textAlign: TextAlign.center,
                        ),
                        const SizedBox(height: 6),
                        Text.rich(
                          TextSpan(
                            children: [
                              const TextSpan(
                                text: 'Driving with Cosmicforge Logistics . ',
                                style: TextStyle(
                                  fontSize: 13,
                                  color: Color(0xFF888888),
                                ),
                              ),
                              TextSpan(
                                text: _membershipDuration(profile.createdAt),
                                style: TextStyle(
                                  fontSize: 13,
                                  fontWeight: FontWeight.w700,
                                  color: Colors.green.shade600,
                                ),
                              ),
                            ],
                          ),
                        ),
                        const SizedBox(height: 2),
                        Text.rich(
                          TextSpan(
                            children: [
                              const TextSpan(
                                text: 'Completed Trips . ',
                                style: TextStyle(
                                  fontSize: 13,
                                  color: Color(0xFF888888),
                                ),
                              ),
                              TextSpan(
                                text: _formatNumber(profile.totalTrips),
                                style: TextStyle(
                                  fontSize: 13,
                                  fontWeight: FontWeight.w700,
                                  color: Colors.green.shade600,
                                ),
                              ),
                            ],
                          ),
                        ),
                        const SizedBox(height: 8),
                        // TODO: confirm the service-type field (Taxi/
                        // Truck/Package) below once exposed by the API —
                        // operationType currently maps to Individual/Fleet.
                        ListenableBuilder(
                          listenable: widget.verificationController,
                          builder: (context, _) {
                            // The verification/status endpoint computes
                            // real-time overall status (including in_progress
                            // and pending_review) and is preferred over the
                            // providers.verification_status DB column, which
                            // is only updated when all steps are approved.
                            final effectiveStatus = widget
                                    .verificationController
                                    .latestStatus
                                    ?.overallStatus ??
                                profile.verificationStatus;
                            return _StatusBadges(
                              verificationStatus: effectiveStatus,
                              operationType: profile.operationType,
                            );
                          },
                        ),
                        const SizedBox(height: 10),
                        _RiderIdRow(supportId: profile.supportId),
                      ],
                    ),
                  ),

                  const SizedBox(height: 24),

                  // ── Stats card ─────────────────────────────────
                  _StatsCard(
                    stats: widget.profileController.stats,
                    isLoading: widget.profileController.isLoading,
                    onRetry: _fetchProfile,
                  ),

                  // ── Menu list ──────────────────────────────────
                  ProfileMenuItem(
                    icon: Icons.person_outline,
                    label: 'Profile Information',
                    onTap: () => Navigator.of(context).push(
                      MaterialPageRoute(
                        builder: (_) => ProfileInfoScreen(
                          profileController: widget.profileController,
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(height: 10),
                  ProfileMenuItem(
                    icon: Icons.map_outlined,
                    label: 'Verification & Documents',
                    onTap: () async {
                      final result = await Navigator.of(context).push<bool>(
                        MaterialPageRoute(
                          builder: (_) => VerificationIntroScreen(
                            verificationController:
                                widget.verificationController,
                            vehicleController: widget.vehicleController,
                            profileController: widget.profileController,
                          ),
                        ),
                      );
                      if (result == true && mounted) {
                        _fetchProfile();
                      }
                    },
                  ),
                  const SizedBox(height: 10),
                  ProfileMenuItem(
                    icon: Icons.people_alt_outlined,
                    label: 'Vehicle Information',
                    onTap: () => Navigator.of(context).push(
                      MaterialPageRoute(
                        builder: (_) => VehicleInformationScreen(
                          vehicleController: widget.vehicleController,
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(height: 10),
                  ProfileMenuItem(
                    icon: Icons.business_outlined,
                    label: 'Safety & Emergency',
                    onTap: () => Navigator.of(context).push(
                      MaterialPageRoute(
                        builder: (_) => SafetyEmergencyScreen(
                          profileController: widget.profileController,
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(height: 10),
                  ProfileMenuItem(
                    icon: Icons.block_outlined,
                    label: 'Security',
                    onTap: () {},
                  ),
                  const SizedBox(height: 10),
                  ProfileMenuItem(
                    icon: Icons.shield_outlined,
                    label: 'Privacy',
                    onTap: () {},
                  ),
                  const SizedBox(height: 10),
                  ProfileMenuItem(
                    icon: Icons.chat_bubble_outline,
                    label: 'Payment & Withdrawals',
                    onTap: () {},
                  ),
                  const SizedBox(height: 10),
                  ProfileMenuItem(
                    icon: Icons.support_agent_outlined,
                    label: 'Support',
                    onTap: () {},
                  ),
                  const SizedBox(height: 10),
                  if (widget.onLogout != null)
                    ProfileMenuItem(
                      icon: Icons.logout,
                      label: _isLoggingOut ? 'Logging out…' : 'Log Out',
                      color: const Color(0xFFE53935),
                      onTap: _isLoggingOut ? null : _confirmLogout,
                      trailing: _isLoggingOut
                          ? const SizedBox(
                              width: 18,
                              height: 18,
                              child: CircularProgressIndicator(
                                strokeWidth: 2,
                                color: Color(0xFFE53935),
                              ),
                            )
                          : null,
                    ),

                  const SizedBox(height: 24),

                  // ── Delete account banner ───────────────────────
                  Container(
                    padding: const EdgeInsets.all(16),
                    decoration: BoxDecoration(
                      color: const Color(0xFFFDECEC),
                      borderRadius: BorderRadius.circular(14),
                    ),
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.center,
                      children: [
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              const Text(
                                'Delete Account',
                                style: TextStyle(
                                  fontSize: 14,
                                  fontWeight: FontWeight.w700,
                                  color: Color(0xFFE53935),
                                ),
                              ),
                              const SizedBox(height: 4),
                              const Text(
                                'When you delete your account, you lose complete access.',
                                style: TextStyle(
                                  fontSize: 12,
                                  color: Color(0xFF888888),
                                  height: 1.4,
                                ),
                              ),
                            ],
                          ),
                        ),
                        const SizedBox(width: 12),
                        ElevatedButton(
                          onPressed: _isDeletingAccount
                              ? null
                              : _confirmDeleteAccount,
                          style: ElevatedButton.styleFrom(
                            backgroundColor: const Color(0xFFE53935),
                            foregroundColor: Colors.white,
                            disabledBackgroundColor:
                                const Color(0xFFE53935).withValues(alpha: 0.6),
                            elevation: 0,
                            padding: const EdgeInsets.symmetric(
                              horizontal: 16,
                              vertical: 12,
                            ),
                            shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(999),
                            ),
                          ),
                          child: _isDeletingAccount
                              ? const SizedBox(
                                  width: 16,
                                  height: 16,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                    color: Colors.white,
                                  ),
                                )
                              : const Text(
                                  'Delete Account',
                                  style: TextStyle(
                                    fontSize: 13,
                                    fontWeight: FontWeight.w700,
                                  ),
                                ),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
      ),
    );
  }
}

/// Displays the rider's support ID (e.g. KG-6BA7B810) with a copy button.
class _RiderIdRow extends StatelessWidget {
  const _RiderIdRow({required this.supportId});
  final String supportId;

  @override
  Widget build(BuildContext context) {
    if (supportId.isEmpty) return const SizedBox.shrink();
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          supportId,
          style: const TextStyle(
            fontSize: 12,
            fontWeight: FontWeight.w600,
            color: Color(0xFF888888),
            letterSpacing: 0.5,
          ),
        ),
        const SizedBox(width: 4),
        GestureDetector(
          onTap: () {
            Clipboard.setData(ClipboardData(text: supportId));
            ScaffoldMessenger.of(context).showSnackBar(
              const SnackBar(
                content: Text('Rider ID copied'),
                duration: Duration(seconds: 2),
              ),
            );
          },
          child: const Icon(
            Icons.copy_outlined,
            size: 14,
            color: Color(0xFF888888),
          ),
        ),
      ],
    );
  }
}

/// Resolves [profilePhotoUrl] from the API into a loadable URL.
///
/// TODO: if the backend ever starts returning relative paths instead of
/// full URLs, prepend the API base URL here (e.g. from `core/config.dart`).
/// Currently `profile_photo_url` from the Go service is expected to already
/// be an absolute URL.
String? _resolvePhotoUrl(String? url) {
  if (url == null || url.isEmpty) return null;
  return url;
}

/// Avatar that shows the provider's uploaded photo, falling back to an
/// initial-letter circle while loading or if no photo is set / it fails
/// to load.
class _ProfileAvatar extends StatelessWidget {
  const _ProfileAvatar({required this.photoUrl, required this.fallbackInitial});

  final String? photoUrl;
  final String fallbackInitial;

  static const double _size = 100;

  Widget _fallback() {
    return Container(
      width: _size,
      height: _size,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: const Color(0xFF4CAF50).withValues(alpha: 0.1),
      ),
      child: Text(
        fallbackInitial,
        style: const TextStyle(
          fontSize: 36,
          fontWeight: FontWeight.bold,
          color: Color(0xFF4CAF50),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final resolvedUrl = _resolvePhotoUrl(photoUrl);
    if (resolvedUrl == null) return _fallback();

    return ClipOval(
      child: Image.network(
        resolvedUrl,
        width: _size,
        height: _size,
        fit: BoxFit.cover,
        errorBuilder: (_, _, _) => _fallback(),
        loadingBuilder: (context, child, progress) {
          if (progress == null) return child;
          return Container(
            width: _size,
            height: _size,
            alignment: Alignment.center,
            color: const Color(0xFF4CAF50).withValues(alpha: 0.1),
            child: const SizedBox(
              width: 24,
              height: 24,
              child: CircularProgressIndicator(
                strokeWidth: 2,
                color: Color(0xFF4CAF50),
              ),
            ),
          );
        },
      ),
    );
  }
}

/// "1 year", "3 months", or "New" based on when the provider account
/// was created.
String _membershipDuration(DateTime? createdAt) {
  if (createdAt == null) return '—';

  final now = DateTime.now();
  final months =
      (now.year - createdAt.year) * 12 + (now.month - createdAt.month);

  if (months < 1) return 'New';
  if (months < 12) return '$months month${months == 1 ? '' : 's'}';

  final years = months ~/ 12;
  return '$years year${years == 1 ? '' : 's'}';
}

/// Formats a count with thousands separators, e.g. 1234 -> "1,234".
String _formatNumber(int value) {
  final digits = value.toString();
  final buffer = StringBuffer();
  for (var i = 0; i < digits.length; i++) {
    if (i > 0 && (digits.length - i) % 3 == 0) buffer.write(',');
    buffer.write(digits[i]);
  }
  return buffer.toString();
}

class _StatusBadges extends StatelessWidget {
  const _StatusBadges({
    required this.verificationStatus,
    required this.operationType,
  });

  final String verificationStatus;
  final String? operationType;

  @override
  Widget build(BuildContext context) {
    final Color verificationColor;
    final String verificationLabel;

    switch (verificationStatus) {
      case 'verified':
        verificationColor = const Color(0xFF2E7D32);
        verificationLabel = 'Verified';
        break;
      case 'pending_review':
        verificationColor = const Color(0xFFF57F17);
        verificationLabel = 'Pending Review';
        break;
      case 'in_progress':
        verificationColor = const Color(0xFF1565C0);
        verificationLabel = 'In Progress';
        break;
      case 'rejected':
        verificationColor = const Color(0xFFC62828);
        verificationLabel = 'Rejected';
        break;
      case 'suspended':
        verificationColor = const Color(0xFF37474F);
        verificationLabel = 'Suspended';
        break;
      default:
        verificationColor = const Color(0xFFE53935);
        verificationLabel = 'Unverified';
    }

    // TODO: replace with the dedicated service-type field (Taxi/Truck/
    // Package) once exposed by the API — operationType currently maps to
    // Individual/Fleet, not the vehicle service type shown in the Figma.
    final operationLabel = (operationType?.isNotEmpty == true)
        ? operationType![0].toUpperCase() + operationType!.substring(1)
        : 'Individual';

    return Text.rich(
      TextSpan(
        children: [
          TextSpan(
            text: verificationLabel,
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w700,
              color: verificationColor,
            ),
          ),
          const TextSpan(
            text: '  •  ',
            style: TextStyle(fontSize: 13, color: Color(0xFFCCCCCC)),
          ),
          TextSpan(
            text: operationLabel,
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w700,
              color: Colors.green.shade600,
            ),
          ),
        ],
      ),
    );
  }
}

class _StatsCard extends StatelessWidget {
  const _StatsCard({
    required this.stats,
    required this.isLoading,
    required this.onRetry,
  });

  final ProviderStats? stats;
  final bool isLoading;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    if (isLoading && stats == null) {
      return Container(
        padding: const EdgeInsets.all(20),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(16),
          boxShadow: [
            BoxShadow(
              color: Colors.black.withValues(alpha: 0.04),
              blurRadius: 8,
              offset: const Offset(0, 2),
            ),
          ],
        ),
        child: const Center(
          child: SizedBox(
            height: 32,
            width: 32,
            child: CircularProgressIndicator(
              strokeWidth: 2,
              color: Color(0xFF4CAF50),
            ),
          ),
        ),
      );
    }

    if (stats == null) {
      return Container(
        padding: const EdgeInsets.all(16),
        decoration: BoxDecoration(
          color: Colors.white,
          borderRadius: BorderRadius.circular(16),
        ),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const Text(
              'Could not load stats.',
              style: TextStyle(color: Color(0xFF888888), fontSize: 13),
            ),
            const SizedBox(width: 8),
            GestureDetector(
              onTap: onRetry,
              child: const Text(
                'Retry',
                style: TextStyle(
                  color: Color(0xFF4CAF50),
                  fontSize: 13,
                  fontWeight: FontWeight.w700,
                ),
              ),
            ),
          ],
        ),
      );
    }

    final hasRatings = stats!.ratingsCount > 0;
    final ratingLabel = hasRatings
        ? stats!.avgRating.toStringAsFixed(2)
        : 'No ratings yet';
    final completionLabel =
        '${(stats!.completionRate * 100).toStringAsFixed(0)}%';

    return Container(
      padding: const EdgeInsets.symmetric(vertical: 18, horizontal: 16),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(16),
        boxShadow: [
          BoxShadow(
            color: Colors.black.withValues(alpha: 0.04),
            blurRadius: 8,
            offset: const Offset(0, 2),
          ),
        ],
      ),
      child: Row(
        children: [
          _StatCell(label: 'Trips', value: _formatNumber(stats!.totalTrips)),
          _StatDivider(),
          _StatCell(
            label: 'Rating',
            value: ratingLabel,
            icon: hasRatings ? Icons.star_rounded : null,
            iconColor: const Color(0xFFFFA000),
          ),
          _StatDivider(),
          _StatCell(label: 'Completion', value: completionLabel),
          _StatDivider(),
          _StatCell(
            label: 'Reviews',
            value: _formatNumber(stats!.ratingsCount),
          ),
        ],
      ),
    );
  }
}

class _StatCell extends StatelessWidget {
  const _StatCell({
    required this.label,
    required this.value,
    this.icon,
    this.iconColor,
  });

  final String label;
  final String value;
  final IconData? icon;
  final Color? iconColor;

  @override
  Widget build(BuildContext context) {
    return Expanded(
      child: Column(
        children: [
          if (icon != null)
            Row(
              mainAxisAlignment: MainAxisAlignment.center,
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(icon, size: 14, color: iconColor),
                const SizedBox(width: 2),
                Text(
                  value,
                  style: const TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w800,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
              ],
            )
          else
            Text(
              value,
              style: const TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w800,
                color: Color(0xFF1A1A1A),
              ),
              textAlign: TextAlign.center,
            ),
          const SizedBox(height: 4),
          Text(
            label,
            style: const TextStyle(fontSize: 11, color: Color(0xFF888888)),
            textAlign: TextAlign.center,
          ),
        ],
      ),
    );
  }
}

class _StatDivider extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Container(
      width: 1,
      height: 36,
      color: const Color(0xFFEEEEEE),
      margin: const EdgeInsets.symmetric(horizontal: 4),
    );
  }
}
