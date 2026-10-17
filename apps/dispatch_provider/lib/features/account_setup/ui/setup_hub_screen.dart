import '../../../shared/widgets/journey_icon.dart';
import 'package:flutter/material.dart';
import '../../../core/theme/theme_extensions.dart';
import '../../../shared/widgets/async_content.dart';
import '../../../shared/widgets/dispatch_screen_frame.dart';
import '../../profile/models/provider_profile_models.dart';
import '../../profile/state/provider_profile_controller.dart';

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
  final String? selectedOperationType;
  @override
  State<SetupHubScreen> createState() => _SetupHubScreenState();
}

class _SetupHubScreenState extends State<SetupHubScreen> {
  bool _isLoading = true;
  String? _loadError;
  static const _steps = [
    ('Basic profile', 'Your personal details', JourneySymbol.profile),
    ('Operation type', 'How you work', JourneySymbol.route),
    ('Emergency contact', 'Someone you trust', JourneySymbol.contact),
    ('Guarantor details', 'Your guarantor information', JourneySymbol.shield),
    ('Profile photo', 'A clear photo of you', JourneySymbol.photo),
    ('Review & submit', 'Check everything together', JourneySymbol.review),
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
        _loadError = error.message;
      }),
    );
  }

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

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final p = widget.profileController.profile;
    final completed = p == null ? <int>{} : _completedSteps(p);
    final current = _currentStep(completed);
    return Scaffold(
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 520),
            child: Column(
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(24, 8, 64, 16),
                  child: Row(
                    children: [
                      DispatchBackButton(onPressed: widget.onBack),
                      const SizedBox(width: 12),
                      Expanded(
                        child: Text(
                          'ACCOUNT SETUP',
                          style: TextStyle(
                            fontSize: 10,
                            letterSpacing: 2,
                            fontWeight: FontWeight.w700,
                            color: c.textMuted,
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
                Expanded(
                  child: _isLoading
                      ? const ContentSkeleton()
                      : _loadError != null
                      ? Padding(
                          padding: const EdgeInsets.all(24),
                          child: ActionNotice(
                            message: _loadError!,
                            isError: true,
                            onRetry: _load,
                          ),
                        )
                      : ListView(
                          padding: const EdgeInsets.fromLTRB(24, 0, 24, 24),
                          children: [
                            DispatchStepProgress(
                              current: current,
                              total: 6,
                              completed: completed,
                            ),
                            const SizedBox(height: 28),
                            Row(
                              crossAxisAlignment: CrossAxisAlignment.center,
                              children: [
                                Expanded(
                                  child: Column(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
                                    children: [
                                      Text(
                                        completed.isEmpty
                                            ? 'Let?s get you\nready to deliver.'
                                            : 'Your journey,\ncontinued.',
                                        style: Theme.of(
                                          context,
                                        ).textTheme.headlineMedium,
                                      ),
                                      const SizedBox(height: 12),
                                      Text(
                                        '${completed.length} of 6 setup details saved',
                                        style: Theme.of(
                                          context,
                                        ).textTheme.bodyMedium,
                                      ),
                                    ],
                                  ),
                                ),
                                const SizedBox(width: 12),
                                ClipRRect(
                                  borderRadius: BorderRadius.circular(24),
                                  child: Image.asset(
                                    'assets/figma/welcome_hero.png',
                                    height: 150,
                                    width: 100,
                                    fit: BoxFit.cover,
                                    alignment: Alignment.topCenter,
                                  ),
                                ),
                              ],
                            ),
                            const SizedBox(height: 24),
                            for (var i = 0; i < _steps.length; i++)
                              Padding(
                                padding: const EdgeInsets.only(bottom: 10),
                                child: Material(
                                  color: c.surface,
                                  shape: RoundedRectangleBorder(
                                    borderRadius: BorderRadius.circular(20),
                                    side: BorderSide(
                                      color: i + 1 == current
                                          ? c.brandTeal600
                                          : c.divider,
                                    ),
                                  ),
                                  child: InkWell(
                                    borderRadius: BorderRadius.circular(20),
                                    onTap:
                                        completed.contains(i + 1) ||
                                            i + 1 == current
                                        ? () => widget.onContinue(i + 1)
                                        : null,
                                    child: Padding(
                                      padding: const EdgeInsets.all(16),
                                      child: Row(
                                        children: [
                                          Container(
                                            width: 46,
                                            height: 46,
                                            decoration: BoxDecoration(
                                              color: completed.contains(i + 1)
                                                  ? c.successSoft
                                                  : c.surfaceElevated,
                                              borderRadius:
                                                  BorderRadius.circular(15),
                                            ),
                                            child: JourneyIcon(
                                              _steps[i].$3,
                                              color: completed.contains(i + 1)
                                                  ? c.success
                                                  : i + 1 == current
                                                  ? c.brandTeal600
                                                  : c.textMuted,
                                              size: 23,
                                            ),
                                          ),
                                          const SizedBox(width: 14),
                                          Expanded(
                                            child: Column(
                                              crossAxisAlignment:
                                                  CrossAxisAlignment.start,
                                              children: [
                                                Text(
                                                  _steps[i].$1,
                                                  style: Theme.of(
                                                    context,
                                                  ).textTheme.titleSmall,
                                                ),
                                                const SizedBox(height: 4),
                                                Text(
                                                  completed.contains(i + 1)
                                                      ? (i == 1
                                                            ? 'Selected'
                                                            : i == 5
                                                            ? 'Submitted'
                                                            : 'Saved')
                                                      : i + 1 == current
                                                      ? _steps[i].$2
                                                      : 'Not started',
                                                  style: Theme.of(
                                                    context,
                                                  ).textTheme.bodyMedium,
                                                ),
                                              ],
                                            ),
                                          ),
                                          const SizedBox(width: 8),
                                          Icon(
                                            completed.contains(i + 1)
                                                ? Icons.check_circle_rounded
                                                : i + 1 == current
                                                ? Icons.chevron_right_rounded
                                                : Icons.lock_outline_rounded,
                                            size: completed.contains(i + 1)
                                                ? 20
                                                : 18,
                                            color: completed.contains(i + 1)
                                                ? c.success
                                                : i + 1 == current
                                                ? c.brandTeal600
                                                : c.textDisabled,
                                          ),
                                        ],
                                      ),
                                    ),
                                  ),
                                ),
                              ),
                          ],
                        ),
                ),
                if (!_isLoading && _loadError == null)
                  Padding(
                    padding: const EdgeInsets.fromLTRB(24, 8, 24, 20),
                    child: SizedBox(
                      width: double.infinity,
                      child: FilledButton(
                        onPressed: () => widget.onContinue(current),
                        child: Row(
                          mainAxisAlignment: MainAxisAlignment.center,
                          children: [
                            Text(
                              completed.isEmpty
                                  ? 'Start setup'
                                  : 'Continue setup',
                            ),
                            const SizedBox(width: 12),
                            const Icon(Icons.arrow_forward_rounded, size: 19),
                          ],
                        ),
                      ),
                    ),
                  ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
