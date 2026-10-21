import 'package:flutter/material.dart';

import '../../design_system/karryam_design_system.dart';

enum KarryamDispatchIntroState { earn, operate, secure }

class KarryamDispatchIntroPreview extends StatelessWidget {
  const KarryamDispatchIntroPreview({super.key, required this.state});

  final KarryamDispatchIntroState state;

  @override
  Widget build(BuildContext context) {
    final data = switch (state) {
      KarryamDispatchIntroState.earn => const _IntroData(
          eyebrow: 'DELIVER WITH KARRYAM',
          title: 'Earn on your terms.',
          body: 'Go online when you’re ready, receive eligible delivery opportunities and build a reliable provider history with every completed mission.',
          icon: Icons.delivery_dining_rounded,
          step: 0,
        ),
      KarryamDispatchIntroState.operate => const _IntroData(
          eyebrow: 'STAY IN CONTROL',
          title: 'Know the mission before you move.',
          body: 'See pickup and delivery context, follow clear mission states, manage multiple stops and always return to your active mission in one tap.',
          icon: Icons.route_rounded,
          step: 1,
        ),
      KarryamDispatchIntroState.secure => const _IntroData(
          eyebrow: 'SECURE EVERY HANDOVER',
          title: 'Move packages with proof.',
          body: 'Karryam protects custody with parcel scans, OTP or QR verification, evidence capture and recovery flows when delivery does not go as planned.',
          icon: Icons.verified_user_rounded,
          step: 2,
        ),
    };

    return Scaffold(
      backgroundColor: KarryamColors.midnightNavy,
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 430),
            child: Padding(
              padding: const EdgeInsets.fromLTRB(24, 20, 24, 28),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Container(
                        width: 36,
                        height: 36,
                        decoration: BoxDecoration(
                          color: KarryamColors.pureWhite,
                          borderRadius: BorderRadius.circular(12),
                        ),
                        alignment: Alignment.center,
                        child: const Text(
                          'K',
                          style: TextStyle(
                            color: KarryamColors.midnightNavy,
                            fontSize: 18,
                            fontWeight: FontWeight.w900,
                          ),
                        ),
                      ),
                      const SizedBox(width: 10),
                      Text(
                        'Karryam',
                        style: Theme.of(context).textTheme.titleMedium?.copyWith(
                              color: KarryamColors.pureWhite,
                              fontWeight: FontWeight.w900,
                            ),
                      ),
                      const Spacer(),
                      TextButton(
                        onPressed: () {},
                        child: const Text('Skip'),
                      ),
                    ],
                  ),
                  const Spacer(),
                  Container(
                    height: 300,
                    width: double.infinity,
                    decoration: BoxDecoration(
                      color: KarryamColors.navy80,
                      borderRadius: BorderRadius.circular(32),
                      border: Border.all(color: KarryamColors.pureWhite.withValues(alpha: .08)),
                    ),
                    child: Stack(
                      children: [
                        Positioned(
                          left: 24,
                          right: 24,
                          top: 38,
                          bottom: 38,
                          child: DecoratedBox(
                            decoration: BoxDecoration(
                              borderRadius: BorderRadius.circular(28),
                              gradient: const LinearGradient(
                                begin: Alignment.topLeft,
                                end: Alignment.bottomRight,
                                colors: [KarryamColors.teal10, KarryamColors.navy60],
                              ),
                            ),
                            child: Center(
                              child: Container(
                                width: 112,
                                height: 112,
                                decoration: const BoxDecoration(
                                  color: KarryamColors.auroraTeal,
                                  shape: BoxShape.circle,
                                ),
                                child: Icon(data.icon, size: 54, color: KarryamColors.midnightNavy),
                              ),
                            ),
                          ),
                        ),
                        Positioned(
                          right: 20,
                          top: 20,
                          child: KarryamStatusChip(
                            label: switch (state) {
                              KarryamDispatchIntroState.earn => 'READY',
                              KarryamDispatchIntroState.operate => 'MISSION-FIRST',
                              KarryamDispatchIntroState.secure => 'PROTECTED',
                            },
                            tone: KarryamStatusTone.active,
                          ),
                        ),
                      ],
                    ),
                  ),
                  const Spacer(),
                  Text(
                    data.eyebrow,
                    style: Theme.of(context).textTheme.labelSmall?.copyWith(
                          color: KarryamColors.auroraTeal,
                          letterSpacing: 1.4,
                          fontWeight: FontWeight.w800,
                        ),
                  ),
                  const SizedBox(height: KarryamSpacing.sm),
                  Text(
                    data.title,
                    style: Theme.of(context).textTheme.headlineLarge?.copyWith(
                          color: KarryamColors.pureWhite,
                          fontWeight: FontWeight.w900,
                          height: 1.05,
                        ),
                  ),
                  const SizedBox(height: KarryamSpacing.sm),
                  Text(
                    data.body,
                    style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                          color: KarryamColors.pureWhite.withValues(alpha: .74),
                          height: 1.5,
                        ),
                  ),
                  const SizedBox(height: KarryamSpacing.xl),
                  Row(
                    children: [
                      ...List.generate(3, (index) => AnimatedContainer(
                            duration: KarryamMotion.fast,
                            width: index == data.step ? 28 : 8,
                            height: 8,
                            margin: const EdgeInsets.only(right: 8),
                            decoration: BoxDecoration(
                              color: index == data.step ? KarryamColors.auroraTeal : KarryamColors.pureWhite.withValues(alpha: .24),
                              borderRadius: BorderRadius.circular(99),
                            ),
                          )),
                      const Spacer(),
                      SizedBox(
                        width: 58,
                        height: 58,
                        child: FilledButton(
                          onPressed: () {},
                          style: FilledButton.styleFrom(
                            backgroundColor: data.step == 2 ? KarryamColors.auroraTeal : KarryamColors.pureWhite,
                            foregroundColor: KarryamColors.midnightNavy,
                            shape: const CircleBorder(),
                            padding: EdgeInsets.zero,
                          ),
                          child: Icon(data.step == 2 ? Icons.check_rounded : Icons.arrow_forward_rounded),
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

class _IntroData {
  const _IntroData({
    required this.eyebrow,
    required this.title,
    required this.body,
    required this.icon,
    required this.step,
  });

  final String eyebrow;
  final String title;
  final String body;
  final IconData icon;
  final int step;
}
