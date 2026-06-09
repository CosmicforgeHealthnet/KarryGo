import 'package:flutter/material.dart';
import '../state/provider_profile_controller.dart';

class ProfileScreen extends StatefulWidget {
  final ProviderProfileController profileController;
  /// Called when the user taps Logout. The caller is responsible for clearing
  /// tokens and routing back to the login screen.
  final VoidCallback? onLogout;

  const ProfileScreen({
    super.key,
    required this.profileController,
    this.onLogout,
  });

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> {
  bool _isLoading = false;
  bool _isLoggingOut = false;

  @override
  void initState() {
    super.initState();
    _fetchProfile();
  }

  Future<void> _fetchProfile() async {
    setState(() => _isLoading = true);
    await widget.profileController.loadMe();
    if (mounted) {
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
            style: TextButton.styleFrom(foregroundColor: const Color(0xFFE53935)),
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

  @override
  Widget build(BuildContext context) {
    final profile = widget.profileController.profile;

    return Scaffold(
      backgroundColor: const Color(0xFFF5F5F5),
      appBar: AppBar(
        title: const Text(
          'My Profile',
          style: TextStyle(
            fontWeight: FontWeight.w800,
            fontSize: 20,
            color: Color(0xFF1A1A1A),
          ),
        ),
        backgroundColor: Colors.white,
        elevation: 0,
        scrolledUnderElevation: 0,
        centerTitle: true,
      ),
      body: _isLoading
          ? const Center(
              child: CircularProgressIndicator(
                color: Color(0xFF4CAF50),
              ),
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
              : SingleChildScrollView(
                  padding: EdgeInsets.fromLTRB(
                    24,
                    24,
                    24,
                    // Extra bottom clearance so the Logout button sits above the
                    // floating bottom nav (70px height + 12px margin = 82px) plus
                    // the system navigation bar inset.
                    140 + MediaQuery.of(context).padding.bottom,
                  ),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      // Header card with name and verification status
                      Container(
                        padding: const EdgeInsets.all(20),
                        decoration: BoxDecoration(
                          color: Colors.white,
                          borderRadius: BorderRadius.circular(16),
                          boxShadow: [
                            BoxShadow(
                              color: Colors.black.withValues(alpha: 0.04),
                              blurRadius: 10,
                              offset: const Offset(0, 4),
                            ),
                          ],
                        ),
                        child: Column(
                          children: [
                            CircleAvatar(
                              radius: 40,
                              backgroundColor: const Color(0xFF4CAF50).withValues(alpha: 0.1),
                              child: Text(
                                (profile.fullName?.isNotEmpty == true)
                                    ? profile.fullName![0].toUpperCase()
                                    : '?',
                                style: const TextStyle(
                                  fontSize: 32,
                                  fontWeight: FontWeight.bold,
                                  color: Color(0xFF4CAF50),
                                ),
                              ),
                            ),
                            const SizedBox(height: 16),
                            Text(
                              profile.fullName ?? 'Unnamed Provider',
                              style: const TextStyle(
                                fontSize: 18,
                                fontWeight: FontWeight.w800,
                                color: Color(0xFF1A1A1A),
                              ),
                              textAlign: TextAlign.center,
                            ),
                            const SizedBox(height: 8),
                            _VerificationBadge(status: profile.verificationStatus),
                          ],
                        ),
                      ),
                      const SizedBox(height: 24),

                      // Info Details List Card
                      const Text(
                        'PERSONAL INFORMATION',
                        style: TextStyle(
                          fontSize: 12,
                          fontWeight: FontWeight.w700,
                          color: Color(0xFF888888),
                          letterSpacing: 1.0,
                        ),
                      ),
                      const SizedBox(height: 8),
                      Container(
                        decoration: BoxDecoration(
                          color: Colors.white,
                          borderRadius: BorderRadius.circular(16),
                          boxShadow: [
                            BoxShadow(
                              color: Colors.black.withValues(alpha: 0.04),
                              blurRadius: 10,
                              offset: const Offset(0, 4),
                            ),
                          ],
                        ),
                        child: Column(
                          children: [
                            _InfoRow(
                              icon: Icons.phone_outlined,
                              label: 'Phone Number',
                              value: profile.phone,
                            ),
                            const Divider(height: 1, color: Color(0xFFEEEEEE)),
                            _InfoRow(
                              icon: Icons.mail_outline_rounded,
                              label: 'Email Address',
                              value: profile.email ?? 'Not provided',
                            ),
                            const Divider(height: 1, color: Color(0xFFEEEEEE)),
                            _InfoRow(
                              icon: Icons.location_on_outlined,
                              label: 'Location',
                              value: (profile.city != null && profile.state != null && profile.city!.isNotEmpty && profile.state!.isNotEmpty)
                                  ? '${profile.city}, ${profile.state}'
                                  : 'Not set',
                            ),
                            const Divider(height: 1, color: Color(0xFFEEEEEE)),
                            _InfoRow(
                              icon: Icons.business_center_outlined,
                              label: 'Operation Mode',
                              value: profile.operationType?.toUpperCase() ?? 'INDIVIDUAL',
                            ),
                          ],
                        ),
                      ),

                      const SizedBox(height: 32),

                      // ── Logout ──────────────────────────────────────
                      if (widget.onLogout != null)
                        SizedBox(
                          height: 52,
                          child: OutlinedButton.icon(
                            onPressed: _isLoggingOut ? null : _confirmLogout,
                            icon: _isLoggingOut
                                ? const SizedBox(
                                    width: 16,
                                    height: 16,
                                    child: CircularProgressIndicator(
                                      strokeWidth: 2,
                                      color: Color(0xFFE53935),
                                    ),
                                  )
                                : const Icon(
                                    Icons.logout_rounded,
                                    size: 18,
                                    color: Color(0xFFE53935),
                                  ),
                            label: Text(
                              _isLoggingOut ? 'Logging out…' : 'Log Out',
                              style: const TextStyle(
                                fontSize: 15,
                                fontWeight: FontWeight.w700,
                                color: Color(0xFFE53935),
                              ),
                            ),
                            style: OutlinedButton.styleFrom(
                              side: const BorderSide(color: Color(0xFFE53935)),
                              shape: RoundedRectangleBorder(
                                borderRadius: BorderRadius.circular(999),
                              ),
                            ),
                          ),
                        ),

                      const SizedBox(height: 24),
                    ],
                  ),
                ),
    );
  }
}

class _InfoRow extends StatelessWidget {
  final IconData icon;
  final String label;
  final String value;

  const _InfoRow({
    required this.icon,
    required this.label,
    required this.value,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
      child: Row(
        children: [
          Icon(icon, size: 20, color: const Color(0xFF4CAF50)),
          const SizedBox(width: 16),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  label,
                  style: const TextStyle(
                    fontSize: 11,
                    color: Color(0xFF888888),
                    fontWeight: FontWeight.w600,
                  ),
                ),
                const SizedBox(height: 4),
                Text(
                  value,
                  style: const TextStyle(
                    fontSize: 14,
                    color: Color(0xFF1A1A1A),
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _VerificationBadge extends StatelessWidget {
  final String status;

  const _VerificationBadge({required this.status});

  @override
  Widget build(BuildContext context) {
    final Color bgColor;
    final Color textColor;
    final String labelText;

    switch (status) {
      case 'verified':
        bgColor = const Color(0xFFE8F5E9);
        textColor = const Color(0xFF2E7D32);
        labelText = 'VERIFIED';
        break;
      case 'pending_review':
        bgColor = const Color(0xFFFFF8E1);
        textColor = const Color(0xFFF57F17);
        labelText = 'PENDING REVIEW';
        break;
      case 'rejected':
        bgColor = const Color(0xFFFFEBEE);
        textColor = const Color(0xFFC62828);
        labelText = 'REJECTED';
        break;
      case 'suspended':
        bgColor = const Color(0xFFECEFF1);
        textColor = const Color(0xFF37474F);
        labelText = 'SUSPENDED';
        break;
      default:
        bgColor = const Color(0xFFF5F5F5);
        textColor = const Color(0xFF616161);
        labelText = 'UNVERIFIED';
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
      decoration: BoxDecoration(
        color: bgColor,
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        labelText,
        style: TextStyle(
          fontSize: 10,
          fontWeight: FontWeight.w800,
          color: textColor,
          letterSpacing: 0.8,
        ),
      ),
    );
  }
}
