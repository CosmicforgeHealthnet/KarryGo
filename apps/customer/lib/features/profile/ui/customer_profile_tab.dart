import 'package:flutter/material.dart';

import '../../../shared/widgets/figma_customer_widgets.dart';
import '../../auth/data/customer_auth_api.dart';
import '../../auth/models/customer_auth_models.dart';
import '../../auth/state/customer_auth_controller.dart';
import '../../media/data/media_upload_service.dart';
import '../../support/data/support_api.dart';
import '../../support/ui/support_chat_screen.dart';
import 'customer_profile_edit_screen.dart';
import 'emergency_contact_screen.dart';

class CustomerProfileTab extends StatelessWidget {
  const CustomerProfileTab({
    super.key,
    required this.profile,
    required this.session,
    required this.api,
    required this.supportApi,
    required this.controller,
    required this.mediaUploadService,
    required this.onProfileUpdated,
  });

  final CustomerProfile profile;
  final CustomerSession session;
  final CustomerAuthApi api;
  final SupportApi supportApi;
  final CustomerAuthController controller;
  final MediaUploadService mediaUploadService;
  final ValueChanged<CustomerProfile> onProfileUpdated;

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.symmetric(horizontal: 20),
      children: [
        // Avatar + Name card
        Padding(
          padding: const EdgeInsets.only(top: 24, bottom: 20),
          child: Column(
            children: [
              CustomerProfileAvatar(photoUrl: profile.photoUrl),
              const SizedBox(height: 14),
              Text(
                profile.displayName,
                style: const TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 20,
                  fontWeight: FontWeight.w900,
                ),
              ),
              const SizedBox(height: 8),
              if (profile.phone.isNotEmpty)
                Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      profile.phone,
                      style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
                    ),
                    const SizedBox(width: 4),
                    const Icon(Icons.verified_rounded, color: CustomerFigmaColors.primary, size: 14),
                  ],
                ),
              if (profile.email.isNotEmpty) ...[
                const SizedBox(height: 4),
                Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      profile.email,
                      style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
                    ),
                    const SizedBox(width: 4),
                    const Icon(Icons.verified_rounded, color: CustomerFigmaColors.primary, size: 14),
                  ],
                ),
              ],
              const SizedBox(height: 18),
              SizedBox(
                width: double.infinity,
                child: ElevatedButton(
                  onPressed: () => Navigator.of(context).push(
                    MaterialPageRoute(
                      builder: (_) => CustomerProfileEditScreen(
                        profile: profile,
                        session: session,
                        api: api,
                        mediaUploadService: mediaUploadService,
                        onSaved: onProfileUpdated,
                      ),
                    ),
                  ),
                  style: ElevatedButton.styleFrom(
                    backgroundColor: CustomerFigmaColors.primary,
                    foregroundColor: Colors.white,
                    elevation: 0,
                    shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(999)),
                    padding: const EdgeInsets.symmetric(vertical: 12),
                    textStyle: const TextStyle(fontSize: 14, fontWeight: FontWeight.w800),
                  ),
                  child: const Row(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      Text('Edit Profile'),
                      SizedBox(width: 4),
                      Icon(Icons.keyboard_arrow_down_rounded, size: 18),
                    ],
                  ),
                ),
              ),
            ],
          ),
        ),

        const SizedBox(height: 16),

        Column(
            children: [
              CustomerProfileMenuItem(
                icon: Icons.map_outlined,
                label: 'Saved Locations',
                onTap: () {},
              ),
              const CustomerProfileDivider(),
              CustomerProfileMenuItem(
                icon: Icons.emergency_outlined,
                label: 'Emergency Contact',
                onTap: () => Navigator.of(context).push(
                  MaterialPageRoute(
                    builder: (_) => EmergencyContactScreen(session: session, api: api),
                  ),
                ),
              ),
              const CustomerProfileDivider(),
              CustomerProfileMenuItem(
                icon: Icons.bar_chart_outlined,
                label: 'Analytics',
                onTap: () {},
              ),
              const CustomerProfileDivider(),
              CustomerProfileMenuItem(
                icon: Icons.security_outlined,
                label: 'Privacy',
                onTap: () {},
              ),
              const CustomerProfileDivider(),
              CustomerProfileMenuItem(
                icon: Icons.shield_outlined,
                label: 'Safety & Security',
                onTap: () {},
              ),
              const CustomerProfileDivider(),
              CustomerProfileMenuItem(
                icon: Icons.chat_bubble_outline_rounded,
                label: 'Support',
                onTap: () => Navigator.of(context).push(
                  MaterialPageRoute(
                    builder: (_) => SupportChatScreen(session: session, supportApi: supportApi),
                  ),
                ),
              ),
            ],
        ),

        const SizedBox(height: 16),

        InkWell(
          onTap: controller.logout,
          child: Padding(
            padding: const EdgeInsets.symmetric(vertical: 16),
            child: Row(
              children: const [
                Icon(Icons.logout_rounded, color: Color(0xFFE53935), size: 22),
                SizedBox(width: 16),
                Expanded(
                  child: Text(
                    'Log Out',
                    style: TextStyle(
                      color: Color(0xFFE53935),
                      fontSize: 14,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                ),
                Icon(Icons.chevron_right_rounded, color: Color(0xFFE53935), size: 20),
              ],
            ),
          ),
        ),

        const SizedBox(height: 16),

        Container(
          padding: const EdgeInsets.all(16),
          decoration: BoxDecoration(
            color: const Color(0xFFFEE2E2),
            borderRadius: BorderRadius.circular(12),
          ),
          child: Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: const [
                    Text(
                      'Delete Account',
                      style: TextStyle(
                        color: Color(0xFFB91C1C),
                        fontSize: 14,
                        fontWeight: FontWeight.w800,
                      ),
                    ),
                    SizedBox(height: 4),
                    Text(
                      'When you delete your account, you\nlose complete access.',
                      style: TextStyle(color: Color(0xFFB91C1C), fontSize: 12),
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 12),
              ElevatedButton(
                onPressed: () => _confirmDeleteAccount(context),
                style: ElevatedButton.styleFrom(
                  backgroundColor: const Color(0xFFE53935),
                  foregroundColor: Colors.white,
                  elevation: 0,
                  shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
                  padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
                  textStyle: const TextStyle(fontSize: 13, fontWeight: FontWeight.w700),
                ),
                child: const Text('Delete Account'),
              ),
            ],
          ),
        ),

        const SizedBox(height: 32),
      ],
    );
  }

  void _confirmDeleteAccount(BuildContext context) {
    showDialog(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Delete Account'),
        content: const Text(
          'Account deletion is coming soon. Please contact support if you need assistance.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(),
            child: const Text('Close'),
          ),
        ],
      ),
    );
  }
}

// ─── Shared profile widgets ───────────────────────────────────────────────────

class CustomerProfileAvatar extends StatelessWidget {
  const CustomerProfileAvatar({super.key, this.photoUrl});

  final String? photoUrl;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 90,
      height: 90,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: CustomerFigmaColors.primaryPale,
        border: Border.all(color: CustomerFigmaColors.primarySoft, width: 3),
      ),
      child: ClipOval(
        child: photoUrl != null && photoUrl!.isNotEmpty
            ? Image.network(
                photoUrl!,
                fit: BoxFit.cover,
                errorBuilder: (_, __, ___) => const _DefaultAvatar(),
              )
            : const _DefaultAvatar(),
      ),
    );
  }
}

class _DefaultAvatar extends StatelessWidget {
  const _DefaultAvatar();

  @override
  Widget build(BuildContext context) {
    return const Icon(Icons.person_rounded, size: 44, color: CustomerFigmaColors.primary);
  }
}

class CustomerProfileMenuItem extends StatelessWidget {
  const CustomerProfileMenuItem({
    super.key,
    required this.icon,
    required this.label,
    required this.onTap,
  });

  final IconData icon;
  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 16),
        child: Row(
          children: [
            Icon(icon, color: CustomerFigmaColors.text, size: 22),
            const SizedBox(width: 16),
            Expanded(
              child: Text(
                label,
                style: const TextStyle(
                  color: CustomerFigmaColors.text,
                  fontSize: 14,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
            const Icon(Icons.chevron_right_rounded, color: CustomerFigmaColors.muted, size: 20),
          ],
        ),
      ),
    );
  }
}

class CustomerProfileDivider extends StatelessWidget {
  const CustomerProfileDivider({super.key});

  @override
  Widget build(BuildContext context) {
    return const Divider(height: 1, color: CustomerFigmaColors.border);
  }
}
