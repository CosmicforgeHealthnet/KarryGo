import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../profile/models/provider_profile_models.dart';
import '../../profile/state/provider_profile_controller.dart';

class WelcomeBackScreen extends StatefulWidget {
  const WelcomeBackScreen({
    super.key,
    required this.profileController,
    required this.onContinueSetup,
    required this.onUseDifferentAccount,
  });

  final ProviderProfileController profileController;
  final VoidCallback onContinueSetup;
  final VoidCallback onUseDifferentAccount;

  @override
  State<WelcomeBackScreen> createState() => _WelcomeBackScreenState();
}

class _WelcomeBackScreenState extends State<WelcomeBackScreen> {
  bool _isLoading = true;
  String? _loadError;

  static const _totalSteps = 6;

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    setState(() {
      _isLoading = true;
      _loadError = null;
    });
    final result = await widget.profileController.loadMe();
    if (!mounted) return;
    result.when(
      success: (_) => setState(() => _isLoading = false),
      failure: (error) => setState(() {
        _isLoading = false;
        _loadError = error.message.isNotEmpty
            ? error.message
            : 'Could not load your profile.';
      }),
    );
  }

  // Same completion rules as SetupHubScreen.
  int _completedSteps(ProviderProfile? p) {
    if (p == null) return 0;
    var count = 0;
    if ((p.fullName?.isNotEmpty ?? false) &&
        (p.state?.isNotEmpty ?? false) &&
        (p.city?.isNotEmpty ?? false)) {
      count++;
    }
    if (p.operationType?.isNotEmpty ?? false) count++;
    if (p.hasEmergencyContact) count++;
    if (p.hasGuarantor) count++;
    if (p.profilePhotoUrl?.isNotEmpty ?? false) count++;
    if (p.onboardingComplete) count++;
    return count;
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: _isLoading
            ? Center(
                child: CircularProgressIndicator(color: c.brandTeal500),
              )
            : _loadError != null
                ? _ErrorView(message: _loadError!, onRetry: _load)
                : _buildBody(c),
      ),
    );
  }

  Widget _buildBody(dynamic c) {
    final profile = widget.profileController.profile;
    final name = profile?.fullName?.isNotEmpty == true
        ? profile!.fullName!
        : 'Karryam Provider';
    final phone = profile?.phone ?? '—';
    final stepsCompleted = _completedSteps(profile);
    final progress = stepsCompleted / _totalSteps;

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const SizedBox(height: 16),
          Center(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                Image.asset(
                  'assets/figma/splash_logo.png',
                  width: 76,
                ),
                const SizedBox(height: 2),
                Text(
                  'Karryam',
                  style: TextStyle(
                    fontSize: 26,
                    fontWeight: FontWeight.w700,
                    color: c.text,
                    letterSpacing: -0.6,
                    height: 1.0,
                  ),
                ),
              ],
            ),
          ),
          const SizedBox(height: 32),
          Text(
            'Welcome back!',
            style: TextStyle(
              fontSize: 30,
              fontWeight: FontWeight.w700,
              color: c.text,
              letterSpacing: -0.4,
              height: 1.15,
            ),
          ),
          const SizedBox(height: 10),
          Text(
            'Continue where you left off.',
            style: TextStyle(
              fontSize: 17,
              fontWeight: FontWeight.w500,
              color: c.text,
              height: 1.4,
            ),
          ),
          const SizedBox(height: 28),

          // ── Identity card ──
          Container(
            padding: const EdgeInsets.all(16),
            decoration: BoxDecoration(
              color: c.surface,
              borderRadius: BorderRadius.circular(16),
              border: Border.all(color: c.border),
            ),
            child: Row(
              children: [
                Container(
                  width: 56,
                  height: 56,
                  decoration: BoxDecoration(
                    color: c.teal10,
                    shape: BoxShape.circle,
                  ),
                  child: Icon(
                    Icons.person_rounded,
                    color: c.brandTeal500,
                    size: 32,
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        name,
                        style: TextStyle(
                          fontSize: 17,
                          fontWeight: FontWeight.w700,
                          color: c.text,
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        phone,
                        style: TextStyle(
                          fontSize: 15,
                          color: c.textMuted,
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),

          const SizedBox(height: 24),

          // ── Progress ──
          Text(
            'Your setup progress',
            style: TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: c.text,
            ),
          ),
          const SizedBox(height: 10),
          ClipRRect(
            borderRadius: BorderRadius.circular(999),
            child: SizedBox(
              height: 8,
              child: Stack(
                children: [
                  Positioned.fill(
                    child: ColoredBox(color: c.navy10),
                  ),
                  FractionallySizedBox(
                    widthFactor: progress,
                    alignment: Alignment.centerLeft,
                    child: ColoredBox(color: c.brandTeal500),
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 8),
          Text(
            '$stepsCompleted of $_totalSteps steps completed',
            style: TextStyle(
              fontSize: 15,
              color: c.textMuted,
            ),
          ),

          const Spacer(),

          SizedBox(
            height: 58,
            child: FilledButton(
              onPressed: widget.onContinueSetup,
              style: FilledButton.styleFrom(
                backgroundColor: c.brandTeal500,
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(12),
                ),
              ),
              child: const Text(
                'Continue setup',
                style: TextStyle(
                  fontSize: 17,
                  fontWeight: FontWeight.w700,
                  color: Color(0xFF081F44),
                ),
              ),
            ),
          ),
          const SizedBox(height: 12),
          SizedBox(
            height: 58,
            child: OutlinedButton(
              onPressed: widget.onUseDifferentAccount,
              style: OutlinedButton.styleFrom(
                side: BorderSide(color: c.border),
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(12),
                ),
              ),
              child: Text(
                'Use a different account',
                style: TextStyle(
                  fontSize: 16,
                  fontWeight: FontWeight.w600,
                  color: c.text,
                ),
              ),
            ),
          ),
          const SizedBox(height: 24),
        ],
      ),
    );
  }
}

class _ErrorView extends StatelessWidget {
  const _ErrorView({required this.message, required this.onRetry});
  final String message;
  final VoidCallback onRetry;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Center(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 32),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.cloud_off_outlined, size: 48, color: c.textDisabled),
            const SizedBox(height: 16),
            Text(
              message,
              style: TextStyle(fontSize: 15, color: c.textMuted),
              textAlign: TextAlign.center,
            ),
            const SizedBox(height: 20),
            OutlinedButton(onPressed: onRetry, child: const Text('Retry')),
          ],
        ),
      ),
    );
  }
}