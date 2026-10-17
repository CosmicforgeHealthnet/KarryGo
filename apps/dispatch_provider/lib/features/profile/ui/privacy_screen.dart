import '../../../core/theme/theme_extensions.dart';
import 'package:flutter/material.dart';

class PrivacyScreen extends StatefulWidget {
  const PrivacyScreen({super.key});

  @override
  State<PrivacyScreen> createState() => _PrivacyScreenState();
}

class _PrivacyScreenState extends State<PrivacyScreen> {
  bool _locationPermission = false;
  bool _notificationPermission = false;
  bool _accountVisibility = false;
  bool _dataPreferences = false;

  String _profileVisibility = 'Everyone';
  String _tripActivityVisibility = 'Only me';
  String _contactVisibility = 'Hidden';

  bool? _saveTripHistory = true;
  bool? _personalizedRecommendations = true;
  bool? _marketingUpdates = true;
  String _dataRetention = '30 days';

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: context.palette.surface,
      body: SafeArea(
        child: ListView(
          padding: const EdgeInsets.fromLTRB(20, 12, 20, 32),
          children: [
            GestureDetector(
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
            const SizedBox(height: 16),
            const Text(
              'Privacy',
              style: TextStyle(
                fontSize: 20,
                fontWeight: FontWeight.w700,
                color: Color(0xFF1A1A1A),
              ),
            ),
            const SizedBox(height: 2),
            const Text(
              'You have rights to your data.',
              style: TextStyle(fontSize: 13, color: Color(0xFF888888)),
            ),
            const SizedBox(height: 24),

            _ToggleCard(
              title: 'Location Permissions',
              subtitle:
                  'We use your location to find nearby customers, estimate prices and make pick ups faster.',
              value: _locationPermission,
              onChanged: (v) => setState(() => _locationPermission = v),
            ),
            const SizedBox(height: 10),
            _ToggleCard(
              title: 'Notification Permissions',
              subtitle:
                  'Get real-time alerts and updates about your trips, earnings updates, promotions and security alert.',
              value: _notificationPermission,
              onChanged: (v) => setState(() => _notificationPermission = v),
            ),
            const SizedBox(height: 10),
            _ToggleCard(
              title: 'Account Visibility',
              subtitle:
                  'Control how your profile and activity appear within the platform.',
              value: _accountVisibility,
              onChanged: (v) => setState(() => _accountVisibility = v),
            ),

            if (_accountVisibility) ...[
              const SizedBox(height: 20),
              _RadioGroup(
                title: 'Profile Visibility',
                subtitle: 'Who can see your profile?',
                options: const [
                  'Everyone',
                  'Only assigned customers',
                  'Support team only',
                  'Private',
                ],
                selected: _profileVisibility,
                onChanged: (v) => setState(() => _profileVisibility = v),
              ),
              const SizedBox(height: 20),
              _RadioGroup(
                title: 'Trip Activity Visibility',
                subtitle: 'Who can view your trip activity?',
                options: const [
                  'Only me',
                  'Only assigned customers',
                  'Support team only',
                ],
                selected: _tripActivityVisibility,
                onChanged: (v) => setState(() => _tripActivityVisibility = v),
              ),
              const SizedBox(height: 20),
              _RadioGroup(
                title: 'Contact Information Visibility',
                subtitle: 'Who can see your contact details?',
                options: const [
                  'Hidden',
                  'Only assigned customers',
                  'Support team only',
                ],
                selected: _contactVisibility,
                onChanged: (v) => setState(() => _contactVisibility = v),
              ),
              const SizedBox(height: 10),
            ] else
              const SizedBox(height: 10),

            _ToggleCard(
              title: 'Data Preferences',
              subtitle:
                  'Manage how your information is used to improve your platform experience.',
              value: _dataPreferences,
              onChanged: (v) => setState(() => _dataPreferences = v),
            ),

            if (_dataPreferences) ...[
              const SizedBox(height: 20),
              _BoolRadioGroup(
                title: 'Save Trip History',
                options: const ['Yes', 'No'],
                selected: _saveTripHistory == true ? 'Yes' : 'No',
                onChanged: (v) => setState(() => _saveTripHistory = v == 'Yes'),
              ),
              const SizedBox(height: 20),
              _BoolRadioGroup(
                title: 'Personalized Recommendations',
                options: const ['On', 'Off'],
                selected: _personalizedRecommendations == true ? 'On' : 'Off',
                onChanged: (v) =>
                    setState(() => _personalizedRecommendations = v == 'On'),
              ),
              const SizedBox(height: 20),
              _BoolRadioGroup(
                title: 'Marketing Updates',
                options: const ['On', 'Off'],
                selected: _marketingUpdates == true ? 'On' : 'Off',
                onChanged: (v) => setState(() => _marketingUpdates = v == 'On'),
              ),
              const SizedBox(height: 20),
              _RadioGroup(
                title: 'Data Retention Preference',
                subtitle: 'How long should trip records be stored',
                options: const ['30 days', '90 days', '6 months', '1 year'],
                selected: _dataRetention,
                onChanged: (v) => setState(() => _dataRetention = v),
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _ToggleCard extends StatelessWidget {
  const _ToggleCard({
    required this.title,
    required this.subtitle,
    required this.value,
    required this.onChanged,
  });

  final String title;
  final String subtitle;
  final bool value;
  final ValueChanged<bool> onChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
      decoration: BoxDecoration(
        border: Border.all(color: const Color(0xFFEEEEEE)),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  title,
                  style: const TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w700,
                    color: Color(0xFF1A1A1A),
                  ),
                ),
                const SizedBox(height: 4),
                Text(
                  subtitle,
                  style: const TextStyle(
                    fontSize: 12,
                    color: Color(0xFF888888),
                    height: 1.4,
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(width: 12),
          Switch(
            value: value,
            onChanged: onChanged,
            activeColor: const Color(0xFF05124D),
          ),
        ],
      ),
    );
  }
}

class _RadioGroup extends StatelessWidget {
  const _RadioGroup({
    required this.title,
    this.subtitle,
    required this.options,
    required this.selected,
    required this.onChanged,
  });

  final String title;
  final String? subtitle;
  final List<String> options;
  final String selected;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          title,
          style: const TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        if (subtitle != null) ...[
          const SizedBox(height: 2),
          Text(
            subtitle!,
            style: const TextStyle(fontSize: 12, color: Color(0xFF888888)),
          ),
        ],
        const SizedBox(height: 10),
        ...options.map(
          (option) => _RadioOption(
            label: option,
            selected: selected == option,
            onTap: () => onChanged(option),
          ),
        ),
      ],
    );
  }
}

class _BoolRadioGroup extends StatelessWidget {
  const _BoolRadioGroup({
    required this.title,
    required this.options,
    required this.selected,
    required this.onChanged,
  });

  final String title;
  final List<String> options;
  final String selected;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          title,
          style: const TextStyle(
            fontSize: 14,
            fontWeight: FontWeight.w700,
            color: Color(0xFF1A1A1A),
          ),
        ),
        const SizedBox(height: 10),
        ...options.map(
          (option) => _RadioOption(
            label: option,
            selected: selected == option,
            onTap: () => onChanged(option),
          ),
        ),
      ],
    );
  }
}

class _RadioOption extends StatelessWidget {
  const _RadioOption({
    required this.label,
    required this.selected,
    required this.onTap,
  });

  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Padding(
        padding: const EdgeInsets.only(bottom: 12),
        child: Row(
          children: [
            Container(
              width: 22,
              height: 22,
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(4),
                color: selected ? const Color(0xFF05124D) : Colors.transparent,
                border: Border.all(color: const Color(0xFF05124D), width: 1.5),
              ),
              child: selected
                  ? const Icon(Icons.check, size: 14, color: Colors.white)
                  : null,
            ),
            const SizedBox(width: 10),
            Text(
              label,
              style: const TextStyle(fontSize: 14, color: Color(0xFF1A1A1A)),
            ),
          ],
        ),
      ),
    );
  }
}
