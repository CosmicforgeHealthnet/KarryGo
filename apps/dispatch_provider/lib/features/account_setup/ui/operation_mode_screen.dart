import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../../core/theme/theme_extensions.dart';

class OperationModeScreen extends StatefulWidget {
  const OperationModeScreen({
    super.key,
    required this.onContinue,
    required this.onBack,
    required this.currentStep,
    required this.totalSteps,
  });

  final ValueChanged<String> onContinue;
  final VoidCallback onBack;
  final int currentStep;
  final int totalSteps;

  @override
  State<OperationModeScreen> createState() => _OperationModeScreenState();
}

class _OperationModeScreenState extends State<OperationModeScreen>
    with SingleTickerProviderStateMixin {
  String? _selected;

  late final AnimationController _entrance = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1000),
  )..forward();

  static const _options = [
    _ModeOption(
      id: 'own_vehicle',
      title: 'Own vehicle',
      subtitle: 'I use my personal vehicle for deliveries',
      icon: Icons.two_wheeler,
    ),
    _ModeOption(
      id: 'company_vehicle',
      title: 'Company vehicle',
      subtitle: 'I operate a company vehicle',
      icon: Icons.local_shipping_outlined,
    ),
    _ModeOption(
      id: 'fleet_operator',
      title: 'Fleet operator',
      subtitle: 'I manage multiple vehicles',
      icon: Icons.garage_outlined,
    ),
    _ModeOption(
      id: 'third_party_driver',
      title: 'Third-party driver',
      subtitle: 'I drive for a registered logistics partner',
      icon: Icons.badge_outlined,
    ),
  ];

  @override
  void dispose() {
    _entrance.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final enabled = _selected != null;

    return Scaffold(
      backgroundColor: c.background,
      body: SafeArea(
        child: Column(
          children: [
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.symmetric(horizontal: 24),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const SizedBox(height: 8),
                    _Reveal(
                      animation: _entrance,
                      begin: 0,
                      child: Row(
                        crossAxisAlignment: CrossAxisAlignment.start,
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
                          const SizedBox(width: 12),
                          Expanded(
                            child: Padding(
                              padding: const EdgeInsets.only(top: 10),
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.stretch,
                                children: [
                                  Text(
                                    'Step ${widget.currentStep} of ${widget.totalSteps}',
                                    textAlign: TextAlign.center,
                                    style: TextStyle(
                                      fontSize: 13,
                                      fontWeight: FontWeight.w500,
                                      color: c.textMuted,
                                      height: 1.2,
                                    ),
                                  ),
                                  const SizedBox(height: 8),
                                  _StepProgress(
                                    current: widget.currentStep,
                                    total: widget.totalSteps,
                                  ),
                                ],
                              ),
                            ),
                          ),
                          const SizedBox(width: 44),
                        ],
                      ),
                    ),
                    const SizedBox(height: 28),
                    _Reveal(
                      animation: _entrance,
                      begin: 0.08,
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.stretch,
                        children: [
                          Text(
                            'Operation type',
                            style: TextStyle(
                              fontSize: 24,
                              fontWeight: FontWeight.w700,
                              color: c.text,
                              letterSpacing: -0.4,
                              height: 1.2,
                            ),
                          ),
                          const SizedBox(height: 10),
                          Text(
                            'Choose how you operate with Karryam.',
                            style: TextStyle(
                              fontSize: 16,
                              fontWeight: FontWeight.w400,
                              color: c.textMuted,
                              height: 1.5,
                            ),
                          ),
                        ],
                      ),
                    ),
                    const SizedBox(height: 28),
                    for (int i = 0; i < _options.length; i++)
                      _Reveal(
                        animation: _entrance,
                        begin: 0.2 + i * 0.1,
                        child: _ModeCard(
                          option: _options[i],
                          isSelected: _selected == _options[i].id,
                          onTap: () {
                            HapticFeedback.selectionClick();
                            setState(() => _selected = _options[i].id);
                          },
                        ),
                      ),
                    const SizedBox(height: 12),
                  ],
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(24, 8, 24, 20),
              child: AnimatedContainer(
                duration: const Duration(milliseconds: 220),
                height: 58,
                width: double.infinity,
                decoration: BoxDecoration(
                  borderRadius: BorderRadius.circular(12),
                  boxShadow: enabled
                      ? [
                          BoxShadow(
                            color: c.brandTeal500.withValues(alpha: 0.20),
                            blurRadius: 20,
                            offset: const Offset(0, 8),
                          ),
                        ]
                      : const [],
                ),
                child: FilledButton(
                  onPressed: enabled
                      ? () => widget.onContinue(_selected!)
                      : null,
                  style: FilledButton.styleFrom(
                    backgroundColor: c.brandTeal500,
                    disabledBackgroundColor:
                        c.brandTeal500.withValues(alpha: 0.45),
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
            ),
          ],
        ),
      ),
    );
  }
}

class _Reveal extends StatelessWidget {
  const _Reveal({
    required this.animation,
    required this.begin,
    required this.child,
  });

  final Animation<double> animation;
  final double begin;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final end = begin + 0.45 > 1 ? 1.0 : begin + 0.45;
    final interval = Interval(begin, end, curve: Curves.easeOutCubic);

    return AnimatedBuilder(
      animation: animation,
      child: child,
      builder: (context, child) {
        final t = interval.transform(animation.value);
        return Opacity(
          opacity: t,
          child: Transform.translate(
            offset: Offset(0, (1 - t) * 16),
            child: child,
          ),
        );
      },
    );
  }
}

class _ModeOption {
  const _ModeOption({
    required this.id,
    required this.title,
    required this.subtitle,
    required this.icon,
  });

  final String id;
  final String title;
  final String subtitle;
  final IconData icon;
}

class _ModeCard extends StatefulWidget {
  const _ModeCard({
    required this.option,
    required this.isSelected,
    required this.onTap,
  });

  final _ModeOption option;
  final bool isSelected;
  final VoidCallback onTap;

  @override
  State<_ModeCard> createState() => _ModeCardState();
}

class _ModeCardState extends State<_ModeCard> {
  bool _pressed = false;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final selected = widget.isSelected;

    final selectedGlow = c.brandTeal500.withValues(alpha: 0.15);
    final cardShadow = c.brandNavy900.withValues(alpha: 0.04);

    return GestureDetector(
      onTap: widget.onTap,
      onTapDown: (_) => setState(() => _pressed = true),
      onTapUp: (_) => setState(() => _pressed = false),
      onTapCancel: () => setState(() => _pressed = false),
      behavior: HitTestBehavior.opaque,
      child: AnimatedScale(
        scale: _pressed ? 0.985 : 1,
        duration: const Duration(milliseconds: 110),
        child: AnimatedContainer(
          duration: const Duration(milliseconds: 220),
          curve: Curves.easeOut,
          margin: const EdgeInsets.only(bottom: 12),
          padding: const EdgeInsets.all(16),
          decoration: BoxDecoration(
            color: selected ? c.teal10 : c.surface,
            borderRadius: BorderRadius.circular(16),
            border: Border.all(
              color: selected ? c.brandTeal500 : c.navy10,
              width: selected ? 1.5 : 1.2,
            ),
            boxShadow: [
              BoxShadow(
                color: selected ? selectedGlow : cardShadow,
                blurRadius: selected ? 18 : 8,
                offset: Offset(0, selected ? 6 : 2),
              ),
            ],
          ),
          child: Row(
            children: [
              AnimatedContainer(
                duration: const Duration(milliseconds: 220),
                curve: Curves.easeOut,
                width: 52,
                height: 52,
                decoration: BoxDecoration(
                  color: selected ? c.brandTeal500 : c.teal10,
                  shape: BoxShape.circle,
                ),
                child: Center(
                  child: TweenAnimationBuilder<Color?>(
                    tween: ColorTween(
                      end: selected ? Colors.white : c.brandTeal500,
                    ),
                    duration: const Duration(milliseconds: 220),
                    builder: (context, color, _) => Icon(
                      widget.option.icon,
                      size: 26,
                      color: color,
                    ),
                  ),
                ),
              ),
              const SizedBox(width: 16),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      widget.option.title,
                      style: TextStyle(
                        fontSize: 17,
                        fontWeight: FontWeight.w700,
                        color: c.text,
                        height: 1.2,
                      ),
                    ),
                    const SizedBox(height: 4),
                    Text(
                      widget.option.subtitle,
                      style: TextStyle(
                        fontSize: 14,
                        fontWeight: FontWeight.w400,
                        color: c.textMuted,
                        height: 1.35,
                      ),
                    ),
                  ],
                ),
              ),
              const SizedBox(width: 12),
              _RadioDot(isSelected: selected),
            ],
          ),
        ),
      ),
    );
  }
}

class _RadioDot extends StatelessWidget {
  const _RadioDot({required this.isSelected});
  final bool isSelected;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return AnimatedContainer(
      duration: const Duration(milliseconds: 220),
      curve: Curves.easeOut,
      width: 24,
      height: 24,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        color: isSelected ? c.brandTeal500 : Colors.transparent,
        border: Border.all(
          color: isSelected ? c.brandTeal500 : c.border,
          width: 1.8,
        ),
      ),
      child: AnimatedScale(
        scale: isSelected ? 1 : 0,
        duration: const Duration(milliseconds: 220),
        curve: Curves.easeOutBack,
        child: const Center(
          child: Icon(Icons.check_rounded, size: 15, color: Colors.white),
        ),
      ),
    );
  }
}

class _StepProgress extends StatelessWidget {
  const _StepProgress({required this.current, required this.total});

  final int current;
  final int total;

  static const double _height = 9;

  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    final fraction =
        total <= 0 ? 0.0 : (current / total).clamp(0.0, 1.0).toDouble();
    final previous =
        total <= 0 ? 0.0 : ((current - 1) / total).clamp(0.0, 1.0).toDouble();
    final radius = BorderRadius.circular(_height / 2);

    return SizedBox(
      height: _height,
      child: Stack(
        children: [
          Container(
            decoration: BoxDecoration(
              color: c.navy10,
              borderRadius: radius,
            ),
          ),
          TweenAnimationBuilder<double>(
            tween: Tween(begin: previous, end: fraction),
            duration: const Duration(milliseconds: 700),
            curve: Curves.easeOutCubic,
            builder: (context, value, _) {
              return FractionallySizedBox(
                alignment: Alignment.centerLeft,
                widthFactor: value,
                heightFactor: 1,
                child: Container(
                  decoration: BoxDecoration(
                    color: c.brandTeal500,
                    borderRadius: radius,
                  ),
                ),
              );
            },
          ),
        ],
      ),
    );
  }
}