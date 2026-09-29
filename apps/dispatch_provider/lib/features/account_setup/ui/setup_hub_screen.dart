import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../../profile/models/provider_profile_models.dart';
import '../../profile/state/provider_profile_controller.dart';

// ═══════════════════════════════════════════════════════════════════════════
// Setup Hub — progress overview
//
// Real progress, driven by ProviderProfile + the in-memory
// `selectedOperationType` (because the backend refuses to store
// operation_type mid-onboarding — see updateReadOnlyFields).
//
//   Step 1  Basic profile      → fullName + state + city
//   Step 2  Operation type     → profile.operationType OR selectedOperationType
//   Step 3  Emergency contact  → hasEmergencyContact
//   Step 4  Guarantor details  → hasGuarantor
//   Step 5  Profile photo      → profilePhotoUrl
//   Step 6  Review & submit    → onboardingComplete
//
// onContinue tells the shell which step to open next (1..6).
// ═══════════════════════════════════════════════════════════════════════════

class SetupHubScreen extends StatefulWidget {
  const SetupHubScreen({
    super.key,
    required this.profileController,
    required this.onContinue,
    required this.onBack,
    this.selectedOperationType,
  });

  final ProviderProfileController profileController;
  final ValueChanged<int> onContinue;
  final VoidCallback onBack;

  /// Value picked on the Operation Type screen in this session. Because the
  /// backend rejects operation_type on PATCH /provider/me, this is the only
  /// place the value exists until submitOnboarding runs on step 6.
  final String? selectedOperationType;

  @override
  State<SetupHubScreen> createState() => _SetupHubScreenState();
}

class _SetupHubScreenState extends State<SetupHubScreen> {
  bool _isLoading = true;
  String? _loadError;

  static const _steps = [
    _SetupStep(
      title: 'Basic profile',
      subtitle: 'Add your personal details',
    ),
    _SetupStep(
      title: 'Operation type',
      subtitle: 'Choose how you operate',
    ),
    _SetupStep(
      title: 'Emergency contact',
      subtitle: 'Add a trusted contact',
    ),
    _SetupStep(
      title: 'Guarantor details',
      subtitle: 'Provide guarantor information',
    ),
    _SetupStep(
      title: 'Profile photo',
      subtitle: 'Take a clear photo',
    ),
    _SetupStep(
      title: 'Review & submit',
      subtitle: 'Check your details',
    ),
  ];

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

  // ── Progress computation ────────────────────────────────────────────

  Set<int> _completedSteps(ProviderProfile p) {
    final s = <int>{};

    // Step 1 — Basic profile
    if ((p.fullName?.isNotEmpty ?? false) &&
        (p.state?.isNotEmpty ?? false) &&
        (p.city?.isNotEmpty ?? false)) {
      s.add(1);
    }

    // Step 2 — Operation type. Backend stores it only via submitOnboarding
    // (step 6). Until then, the in-memory value is the source of truth.
    final opType = p.operationType;
    final inMemory = widget.selectedOperationType;
    if ((opType?.isNotEmpty ?? false) || (inMemory?.isNotEmpty ?? false)) {
      s.add(2);
    }

    // Step 3 — Emergency contact
    if (p.hasEmergencyContact) s.add(3);

    // Step 4 — Guarantor
    if (p.hasGuarantor) s.add(4);

    // Step 5 — Profile photo
    if (p.profilePhotoUrl?.isNotEmpty ?? false) s.add(5);

    // Step 6 — Review & submit
    if (p.onboardingComplete) s.add(6);

    return s;
  }

  int _currentStep(Set<int> completed) {
    for (var i = 1; i <= 6; i++) {
      if (!completed.contains(i)) return i;
    }
    return 6;
  }

  // ── Build ───────────────────────────────────────────────────────────

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
    final completed = profile == null ? <int>{} : _completedSteps(profile);
    final current = _currentStep(completed);

    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 24),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          const SizedBox(height: 8),
          Row(
            children: [
              GestureDetector(
                onTap: widget.onBack,
                behavior: HitTestBehavior.opaque,
                child: Padding(
                  padding: const EdgeInsets.symmetric(
                    vertical: 8,
                    horizontal: 4,
                  ),
                  child: Icon(
                    Icons.arrow_back_rounded,
                    size: 24,
                    color: c.text,
                  ),
                ),
              ),
              const Spacer(),
              GestureDetector(
                onTap: () {},
                behavior: HitTestBehavior.opaque,
                child: Padding(
                  padding: const EdgeInsets.all(6),
                  child: Icon(
                    Icons.more_vert_rounded,
                    size: 22,
                    color: c.text,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 20),
          FittedBox(
            fit: BoxFit.scaleDown,
            alignment: Alignment.centerLeft,
            child: Text(
              'Setup your provider account',
              maxLines: 1,
              style: TextStyle(
                fontSize: 24,
                fontWeight: FontWeight.w700,
                color: c.text,
                letterSpacing: -0.4,
                height: 1.2,
              ),
            ),
          ),
          const SizedBox(height: 8),
          ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 265),
            child: Text(
              'A few steps to get you verified and ready for opportunities.',
              style: TextStyle(
                fontSize: 16,
                fontWeight: FontWeight.w400,
                color: c.textMuted,
                height: 1.45,
              ),
            ),
          ),
          const SizedBox(height: 24),
          Expanded(
            child: SingleChildScrollView(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  for (int i = 0; i < _steps.length; i++) ...[
                    _SetupRow(
                      step: _steps[i],
                      number: i + 1,
                      isCompleted: completed.contains(i + 1),
                      isActive: i + 1 == current,
                    ),
                    if (i < _steps.length - 1) const SizedBox(height: 10),
                  ],
                ],
              ),
            ),
          ),
          const SizedBox(height: 16),
          SizedBox(
            height: 58,
            child: FilledButton(
              onPressed: () => widget.onContinue(current),
              style: FilledButton.styleFrom(
                backgroundColor: c.brandTeal500,
                shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(12),
                ),
              ),
              child: const Text(
                'Continue',
                style: TextStyle(
                  fontSize: 17,
                  fontWeight: FontWeight.w700,
                  color: Colors.white,
                ),
              ),
            ),
          ),
          const SizedBox(height: 20),
        ],
      ),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Step row
// ═══════════════════════════════════════════════════════════════════════════

class _SetupStep {
  const _SetupStep({required this.title, required this.subtitle});
  final String title;
  final String subtitle;
}

class _SetupRow extends StatelessWidget {
  const _SetupRow({
    required this.step,
    required this.number,
    required this.isCompleted,
    required this.isActive,
  });

  final _SetupStep step;
  final int number;
  final bool isCompleted;
  final bool isActive;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final activeTint = c.brandTeal500.withValues(alpha: 0.08);
    final activeBorder = c.brandTeal500.withValues(alpha: 0.35);
    final activeGlow = c.brandTeal500.withValues(alpha: 0.12);

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 18, vertical: 20),
      decoration: BoxDecoration(
        color: isActive ? activeTint : c.surface,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(
          color: isActive ? activeBorder : c.divider,
          width: isActive ? 1.4 : 1,
        ),
        boxShadow: [
          BoxShadow(
            color: isActive ? activeGlow : c.cardShadow.first.color,
            blurRadius: isActive ? 14 : 8,
            offset: Offset(0, isActive ? 4 : 2),
          ),
        ],
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          _StepBadge(
            number: number,
            isCompleted: isCompleted,
            isActive: isActive,
          ),
          const SizedBox(width: 16),
          Expanded(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  step.title,
                  style: TextStyle(
                    fontSize: 18,
                    fontWeight: FontWeight.w700,
                    color: c.text,
                    height: 1.2,
                  ),
                ),
                const SizedBox(height: 4),
                Text(
                  step.subtitle,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w400,
                    color: c.textMuted,
                    height: 1.3,
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

class _StepBadge extends StatelessWidget {
  const _StepBadge({
    required this.number,
    required this.isCompleted,
    required this.isActive,
  });

  final int number;
  final bool isCompleted;
  final bool isActive;

  static const double _size = 46;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;

    if (isCompleted) {
      return Container(
        width: _size,
        height: _size,
        decoration: BoxDecoration(
          color: c.brandTeal500,
          shape: BoxShape.circle,
        ),
        child: const Icon(
          Icons.check_rounded,
          size: 24,
          color: Colors.white,
        ),
      );
    }

    if (isActive) {
      return Container(
        width: _size,
        height: _size,
        decoration: BoxDecoration(
          color: c.brandNavy900,
          shape: BoxShape.circle,
        ),
        child: Center(
          child: Text(
            '$number',
            style: const TextStyle(
              fontSize: 18,
              fontWeight: FontWeight.w700,
              color: Colors.white,
            ),
          ),
        ),
      );
    }

    return Container(
      width: _size,
      height: _size,
      decoration: BoxDecoration(
        color: c.surface,
        shape: BoxShape.circle,
        border: Border.all(
          color: c.border,
          width: 1.2,
        ),
      ),
      child: Center(
        child: Text(
          '$number',
          style: TextStyle(
            fontSize: 18,
            fontWeight: FontWeight.w600,
            color: c.textMuted,
          ),
        ),
      ),
    );
  }
}

// ═══════════════════════════════════════════════════════════════════════════
// Error view
// ═══════════════════════════════════════════════════════════════════════════

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