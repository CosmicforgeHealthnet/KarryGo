import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../state/provider_profile_controller.dart';
import 'edit_profile_screen.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Figma 12 — Profile Detail
// Avatar · name · email · rows (Full name / Phone / DOB / Address) ·
// bottom "Edit profile" button.
// ═══════════════════════════════════════════════════════════════════════════

class ProfileInfoScreen extends StatefulWidget {
  const ProfileInfoScreen({super.key, required this.profileController});

  final ProviderProfileController profileController;

  @override
  State<ProfileInfoScreen> createState() => _ProfileInfoScreenState();
}

class _ProfileInfoScreenState extends State<ProfileInfoScreen> {
  @override
  void initState() {
    super.initState();
    widget.profileController.addListener(_onChanged);
  }

  @override
  void dispose() {
    widget.profileController.removeListener(_onChanged);
    super.dispose();
  }

  void _onChanged() {
    if (mounted) setState(() {});
  }

  Future<void> _openEdit() async {
    await Navigator.of(context).push(
      MaterialPageRoute(
        builder: (_) => EditProfileScreen(
          profileController: widget.profileController,
        ),
      ),
    );
    if (mounted) setState(() {});
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final profile = widget.profileController.profile;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            // ── Header: back only ──
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
              child: Row(
                children: [
                  GestureDetector(
                    onTap: () => Navigator.of(context).pop(),
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
                ],
              ),
            ),

            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                children: [
                  const SizedBox(height: 8),

                  Center(
                    child: _Avatar(
                      photoUrl: profile?.profilePhotoUrl,
                      fallbackInitial:
                          (profile?.fullName?.isNotEmpty == true)
                              ? profile!.fullName![0].toUpperCase()
                              : 'K',
                    ),
                  ),
                  const SizedBox(height: 16),

                  Text(
                    profile?.fullName ?? '—',
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      fontSize: 20,
                      fontWeight: FontWeight.w800,
                      color: c.text,
                      letterSpacing: -0.3,
                    ),
                  ),
                  const SizedBox(height: 4),

                  Text(
                    (profile?.email?.isNotEmpty == true)
                        ? profile!.email!
                        : 'no-email@karryam.com',
                    textAlign: TextAlign.center,
                    style: TextStyle(
                      fontSize: 14,
                      fontWeight: FontWeight.w500,
                      color: c.textMuted,
                    ),
                  ),
                  const SizedBox(height: 36),

                  _InfoRow(
                    label: 'Full name',
                    value: profile?.fullName ?? '',
                  ),
                  const SizedBox(height: 22),
                  _InfoRow(
                    label: 'Phone number',
                    value: profile?.phone ?? '',
                  ),
                  const SizedBox(height: 22),
                  const _InfoRow(
                    label: 'Date of birth',
                    value: '—',
                  ),
                  const SizedBox(height: 22),
                  _InfoRow(
                    label: 'Address',
                    value: _address(profile?.city, profile?.state),
                  ),
                ],
              ),
            ),

            // ── Edit profile button ──
            Padding(
              padding: const EdgeInsets.fromLTRB(20, 8, 20, 16),
              child: SizedBox(
                height: 52,
                width: double.infinity,
                child: FilledButton(
                  onPressed: _openEdit,
                  style: FilledButton.styleFrom(
                    textStyle: const TextStyle(
                      fontSize: 16,
                      fontWeight: FontWeight.w700,
                    ),
                  ),
                  child: const Text('Edit profile'),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  static String _address(String? city, String? state) {
    final parts = <String>[];
    if (city != null && city.isNotEmpty) parts.add(city);
    if (state != null && state.isNotEmpty) {
      parts.add(state == 'Federal Capital Territory' ? 'Abuja (FCT)' : state);
    }
    return parts.isEmpty ? '' : parts.join(', ');
  }
}

class _Avatar extends StatelessWidget {
  const _Avatar({required this.photoUrl, required this.fallbackInitial});

  final String? photoUrl;
  final String fallbackInitial;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    const size = 96.0;

    Widget fallback = Container(
      width: size,
      height: size,
      alignment: Alignment.center,
      color: c.teal10,
      child: Text(
        fallbackInitial,
        style: TextStyle(
          fontSize: 34,
          fontWeight: FontWeight.w800,
          color: c.brandTeal600,
        ),
      ),
    );

    if (photoUrl == null || photoUrl!.isEmpty) {
      return Container(
        width: size,
        height: size,
        decoration: const BoxDecoration(shape: BoxShape.circle),
        child: ClipOval(child: fallback),
      );
    }

    return Container(
      width: size,
      height: size,
      decoration: const BoxDecoration(shape: BoxShape.circle),
      child: ClipOval(
        child: Image.network(
          photoUrl!,
          width: size,
          height: size,
          fit: BoxFit.cover,
          errorBuilder: (_, _, _) => fallback,
          loadingBuilder: (context, child, progress) {
            if (progress == null) return child;
            return fallback;
          },
        ),
      ),
    );
  }
}

class _InfoRow extends StatelessWidget {
  const _InfoRow({required this.label, required this.value});
  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: TextStyle(
            fontSize: 13,
            fontWeight: FontWeight.w500,
            color: c.textMuted,
            letterSpacing: 0.2,
          ),
        ),
        const SizedBox(height: 4),
        Text(
          value.isEmpty ? '—' : value,
          style: TextStyle(
            fontSize: 15,
            fontWeight: FontWeight.w700,
            color: c.text,
            letterSpacing: -0.2,
            height: 1.3,
          ),
        ),
      ],
    );
  }
}