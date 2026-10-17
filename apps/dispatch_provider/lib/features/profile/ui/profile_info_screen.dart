import '../../../shared/widgets/account_components.dart';
import '../../../shared/widgets/delivery_collection_widgets.dart';
import '../../../shared/widgets/journey_icon.dart';
import 'package:flutter/material.dart';

import '../state/provider_profile_controller.dart';
import 'edit_profile_screen.dart';

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
        builder: (_) =>
            EditProfileScreen(profileController: widget.profileController),
      ),
    );
    if (mounted) setState(() {});
  }

  @override
  Widget build(BuildContext context) {
    final profile = widget.profileController.profile;
    return AccountPage(
      title: 'Profile details',
      symbol: JourneySymbol.profile,
      description: 'The personal details linked to your rider account.',
      bottom: FilledButton(
        onPressed: _openEdit,
        child: const Text('Edit profile'),
      ),
      children: [
        DeliveryPanel(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              AccountValue(label: 'Full name', value: profile?.fullName ?? ''),
              const Divider(height: 1),
              AccountValue(label: 'Email', value: profile?.email ?? ''),
              const Divider(height: 1),
              AccountValue(label: 'Phone number', value: profile?.phone ?? ''),
              const Divider(height: 1),
              AccountValue(
                label: 'Location',
                value: _address(profile?.city, profile?.state),
              ),
              if (profile?.supportId.isNotEmpty == true) ...[
                const Divider(height: 1),
                AccountValue(label: 'Support ID', value: profile!.supportId),
              ],
            ],
          ),
        ),
      ],
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
