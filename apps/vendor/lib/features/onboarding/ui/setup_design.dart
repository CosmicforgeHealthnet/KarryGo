import 'dart:math' as math;
import 'package:flutter/material.dart';
import '../../../core/theme/vendor_theme.dart';
import '../state/setup_progress.dart';

class SetupSectionLabel extends StatelessWidget {
  const SetupSectionLabel({
    super.key,
    required this.label,
    required this.caption,
  });
  final String label, caption;
  @override
  Widget build(BuildContext context) => Padding(
    padding: const EdgeInsets.only(top: 12, bottom: 16),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: TextStyle(
            fontSize: 10,
            letterSpacing: 2,
            fontWeight: FontWeight.w700,
            color: KarryamColors.tealInk.forInk(context),
          ),
        ),
        const SizedBox(height: 8),
        Text(
          caption,
          style: const TextStyle(
            fontSize: 20,
            letterSpacing: -.6,
            fontWeight: FontWeight.w700,
          ),
        ),
      ],
    ),
  );
}

class SetupJourneyRow extends StatelessWidget {
  const SetupJourneyRow({
    super.key,
    required this.title,
    required this.subtitle,
    required this.icon,
    required this.progress,
    this.onTap,
  });
  final String title, subtitle;
  final IconData icon;
  final SetupProgress progress;
  final VoidCallback? onTap;
  @override
  Widget build(BuildContext context) => Material(
    color: Colors.transparent.forSurface(context),
    child: InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(16),
      child: Padding(
        padding: const EdgeInsets.symmetric(vertical: 18, horizontal: 2),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Container(
              width: 44,
              height: 48,
              decoration: BoxDecoration(
                color: (setupStateColor(
                  progress.state,
                ).withValues(alpha: .09)).forSurface(context),
                borderRadius: BorderRadius.circular(22),
              ),
              child: AnimatedSwitcher(
                duration: motionDuration(context),
                child: Icon(
                  progress.state == SetupState.complete
                      ? Icons.check_rounded
                      : progress.state == SetupState.attention
                      ? Icons.priority_high_rounded
                      : icon,
                  key: ValueKey(progress.state),
                  size: 23,
                  color: (setupStateColor(progress.state)).forInk(context),
                ),
              ),
            ),
            const SizedBox(width: 16),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    title,
                    style: const TextStyle(
                      fontWeight: FontWeight.w700,
                      fontSize: 16,
                      letterSpacing: -.4,
                    ),
                  ),
                  const SizedBox(height: 5),
                  Text(
                    subtitle,
                    style: TextStyle(
                      fontSize: 12,
                      color: KarryamColors.indigo.forInk(context),
                      height: 1.5,
                    ),
                  ),
                  const SizedBox(height: 9),
                  AnimatedSwitcher(
                    duration: motionDuration(context),
                    child: Align(
                      key: ValueKey(progress.state),
                      alignment: Alignment.centerLeft,
                      child: Text(
                        progress.label,
                        style: TextStyle(
                          fontSize: 11,
                          fontWeight: FontWeight.w700,
                          color: (setupStateColor(
                            progress.state,
                          )).forInk(context),
                        ),
                      ),
                    ),
                  ),
                  if (progress.detail != null) ...[
                    const SizedBox(height: 4),
                    Text(
                      progress.detail!,
                      style: TextStyle(
                        fontSize: 11,
                        height: 1.5,
                        color: (setupStateColor(
                          progress.state,
                        )).forInk(context),
                      ),
                    ),
                  ],
                  const SizedBox(height: 18),
                  Divider(
                    height: 1,
                    color: Color(0xFFDEE4DE).forOutline(context),
                  ),
                ],
              ),
            ),
            const SizedBox(width: 10),
            Padding(
              padding: EdgeInsets.only(top: 10),
              child: Icon(
                Icons.arrow_outward_rounded,
                size: 19,
                color: KarryamColors.tealInk.forInk(context),
              ),
            ),
          ],
        ),
      ),
    ),
  );
}

class SetupPassport extends StatelessWidget {
  const SetupPassport({
    super.key,
    required this.progress,
    required this.status,
    required this.caption,
  });
  final double progress;
  final String status, caption;
  @override
  Widget build(BuildContext context) => ClipRRect(
    borderRadius: BorderRadius.circular(26),
    child: Container(
      color: KarryamColors.navy.forSurface(context),
      child: Column(
        children: [
          SizedBox(
            height: 166,
            child: Stack(
              children: [
                Positioned.fill(
                  child: Image.asset(
                    'assets/onboarding/vendor-welcome.png',
                    fit: BoxFit.cover,
                    alignment: const Alignment(.1, -.5),
                  ),
                ),
                Positioned.fill(
                  child: DecoratedBox(
                    decoration: BoxDecoration(
                      gradient: LinearGradient(
                        begin: Alignment.centerLeft,
                        end: Alignment.centerRight,
                        colors: [
                          (KarryamColors.navy.withValues(
                            alpha: .97,
                          )).forSurface(context),
                          (KarryamColors.navy.withValues(
                            alpha: .32,
                          )).forSurface(context),
                        ],
                      ),
                    ),
                  ),
                ),
                Positioned(
                  left: 20,
                  top: 22,
                  bottom: 20,
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    mainAxisAlignment: MainAxisAlignment.spaceBetween,
                    children: [
                      Text(
                        'BUSINESS SETUP',
                        style: TextStyle(
                          color: KarryamColors.teal.forInk(context),
                          fontSize: 10,
                          letterSpacing: 2,
                          fontWeight: FontWeight.w700,
                        ),
                      ),
                      Semantics(
                        label: '${progress.round()} percent complete',
                        child: SizedBox(
                          width: 92,
                          height: 92,
                          child: TweenAnimationBuilder<double>(
                            tween: Tween(begin: 0, end: progress),
                            duration: const Duration(milliseconds: 650),
                            builder: (context, value, _) => CustomPaint(
                              painter: _ProgressOrbit(value),
                              child: Center(
                                child: Text(
                                  '${value.round()}%',
                                  style: TextStyle(
                                    fontSize: 26,
                                    letterSpacing: -1,
                                    fontWeight: FontWeight.w700,
                                    color: Colors.white.forInk(context),
                                  ),
                                ),
                              ),
                            ),
                          ),
                        ),
                      ),
                    ],
                  ),
                ),
              ],
            ),
          ),
          Padding(
            padding: const EdgeInsets.all(20),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Expanded(
                      child: Text(
                        status,
                        style: TextStyle(
                          color: Colors.white.forInk(context),
                          fontWeight: FontWeight.w700,
                          fontSize: 14,
                        ),
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 8),
                Text(
                  caption,
                  style: TextStyle(
                    color: Color(0xFFB9C9D3).forInk(context),
                    height: 1.5,
                    fontSize: 12,
                  ),
                ),
              ],
            ),
          ),
        ],
      ),
    ),
  );
}

class _ProgressOrbit extends CustomPainter {
  _ProgressOrbit(this.progress);
  final double progress;
  @override
  void paint(Canvas canvas, Size size) {
    final rect = Offset.zero & size;
    final arc = rect.deflate(5);
    canvas.drawArc(
      arc,
      -math.pi / 2,
      math.pi * 2,
      false,
      Paint()
        ..color = Colors.white.withValues(alpha: .15)
        ..style = PaintingStyle.stroke
        ..strokeWidth = 3,
    );
    canvas.drawArc(
      arc,
      -math.pi / 2,
      math.pi * 2 * progress / 100,
      false,
      Paint()
        ..color = KarryamColors.teal
        ..style = PaintingStyle.stroke
        ..strokeCap = StrokeCap.round
        ..strokeWidth = 4,
    );
  }

  @override
  bool shouldRepaint(_ProgressOrbit oldDelegate) =>
      oldDelegate.progress != progress;
}

class SetupChapterHeader extends StatelessWidget {
  const SetupChapterHeader({super.key, required this.kind, this.title});
  final String kind;
  final String? title;
  @override
  Widget build(BuildContext context) {
    final (heading, caption, icon, tag) = switch (kind) {
      'business' => (
        'Make it yours.',
        'Your business name, contact details and operating preferences.',
        Icons.storefront_outlined,
        'THE FOUNDATION',
      ),
      'representative' => (
        'The person behind the business.',
        'Introduce your authorized representative.',
        Icons.person_outline_rounded,
        'THE PEOPLE',
      ),
      'bank' => (
        'Your business account.',
        'Protected with a security code. Only masked account details are saved in the app.',
        Icons.account_balance_outlined,
        'THE FINANCES',
      ),
      'locations' => (
        'Built for your business.',
        'Bring your brands and branches together before location verification.',
        Icons.location_on_outlined,
        'THE PLACES',
      ),
      'branch' => (
        'A place to connect.',
        'Give this branch its own name, address and location.',
        Icons.location_on_outlined,
        'THE PLACES',
      ),
      'brand' => (
        'Give your brand a home.',
        'The name your customers will recognize.',
        Icons.storefront_outlined,
        'THE IDENTITY',
      ),
      'capabilities' => (
        'What does your business do?',
        'Choose from the available business types.',
        Icons.category_outlined,
        'THE POSSIBILITIES',
      ),
      'verification' => (
        'Trust starts here.',
        'Add clear documents, submit for review and follow each decision.',
        Icons.verified_user_outlined,
        'THE CONFIDENCE',
      ),
      _ => (
        'Branch settings',
        'Set up how this branch serves your customers.',
        Icons.tune_rounded,
        'THE EVERYDAY',
      ),
    };
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SizedBox(
          height: 112,
          child: Stack(
            children: [
              Positioned(
                right: 4,
                top: 0,
                child: Transform.rotate(
                  angle: -.16,
                  child: Container(
                    width: 96,
                    height: 96,
                    decoration: BoxDecoration(
                      color: Color(0xFFDCEEE5).forSurface(context),
                      borderRadius: BorderRadius.circular(30),
                    ),
                  ),
                ),
              ),
              Positioned(
                right: 15,
                top: 9,
                child: Transform.rotate(
                  angle: .12,
                  child: Container(
                    width: 84,
                    height: 84,
                    decoration: BoxDecoration(
                      color: KarryamColors.navy.forSurface(context),
                      borderRadius: BorderRadius.circular(26),
                    ),
                    child: Icon(
                      icon,
                      color: KarryamColors.teal.forInk(context),
                      size: 36,
                    ),
                  ),
                ),
              ),
              Positioned(
                left: 0,
                bottom: 18,
                child: Text(
                  tag,
                  style: TextStyle(
                    color: KarryamColors.tealInk.forInk(context),
                    fontSize: 10,
                    letterSpacing: 2,
                    fontWeight: FontWeight.w700,
                  ),
                ),
              ),
              Positioned(
                right: 0,
                bottom: 12,
                child: Container(
                  width: 26,
                  height: 26,
                  decoration: BoxDecoration(
                    color: KarryamColors.teal.forSurface(context),
                    shape: BoxShape.circle,
                  ),
                  child: Icon(
                    Icons.add_rounded,
                    size: 17,
                    color: KarryamColors.navy.forInk(context),
                  ),
                ),
              ),
            ],
          ),
        ),
        Text(
          title ?? heading,
          style: const TextStyle(
            fontSize: 36,
            height: 1.08,
            letterSpacing: -1.6,
            fontWeight: FontWeight.w700,
          ),
        ),
        const SizedBox(height: 14),
        Text(
          caption,
          style: TextStyle(
            fontSize: 14,
            color: KarryamColors.indigo.forInk(context),
            height: 1.6,
          ),
        ),
        const SizedBox(height: 24),
        Divider(height: 1, color: Color(0xFFDEE4DE).forOutline(context)),
      ],
    );
  }
}

class SetupFormSurface extends StatelessWidget {
  const SetupFormSurface({super.key, required this.child});
  final Widget child;
  @override
  Widget build(BuildContext context) => Theme(
    data: Theme.of(context).copyWith(
      inputDecorationTheme: InputDecorationTheme(
        filled: false,
        contentPadding: EdgeInsets.fromLTRB(0, 18, 0, 18),
        labelStyle: TextStyle(
          color: KarryamColors.indigo.forInk(context),
          fontSize: 14,
        ),
        floatingLabelStyle: TextStyle(
          color: KarryamColors.tealInk.forInk(context),
          fontWeight: FontWeight.w700,
        ),
        border: UnderlineInputBorder(
          borderSide: BorderSide(color: Color(0xFFCAD5CC).forOutline(context)),
        ),
        enabledBorder: UnderlineInputBorder(
          borderSide: BorderSide(color: Color(0xFFCAD5CC).forOutline(context)),
        ),
        focusedBorder: UnderlineInputBorder(
          borderSide: BorderSide(
            color: KarryamColors.tealInk.forOutline(context),
            width: 2,
          ),
        ),
      ),
      listTileTheme: ListTileThemeData(
        contentPadding: EdgeInsets.symmetric(vertical: 6),
        iconColor: KarryamColors.tealInk.forInk(context),
      ),
    ),
    child: child,
  );
}

class SetupDocumentPicker extends StatelessWidget {
  const SetupDocumentPicker({super.key, this.fileName, this.onPressed});
  final String? fileName;
  final VoidCallback? onPressed;
  @override
  Widget build(BuildContext context) => Material(
    color: Color(0xFFE6EEE8).forSurface(context),
    borderRadius: BorderRadius.circular(22),
    child: InkWell(
      onTap: onPressed,
      borderRadius: BorderRadius.circular(22),
      child: Padding(
        padding: const EdgeInsets.all(22),
        child: Column(
          children: [
            Container(
              width: 52,
              height: 60,
              decoration: BoxDecoration(
                color: Colors.white.forSurface(context),
                borderRadius: BorderRadius.circular(12),
              ),
              child: Icon(
                fileName == null
                    ? Icons.upload_file_outlined
                    : Icons.task_outlined,
                color: KarryamColors.tealInk.forInk(context),
                size: 28,
              ),
            ),
            const SizedBox(height: 16),
            Text(
              fileName ?? 'Choose document',
              textAlign: TextAlign.center,
              style: const TextStyle(fontSize: 15, fontWeight: FontWeight.w700),
            ),
            const SizedBox(height: 8),
            Text(
              'JPEG, PNG, WebP or PDF / Up to 10 MB',
              textAlign: TextAlign.center,
              style: TextStyle(
                fontSize: 11,
                color: KarryamColors.indigo.forInk(context),
                height: 1.5,
              ),
            ),
            if (fileName != null)
              Padding(
                padding: EdgeInsets.only(top: 8),
                child: Text(
                  'Tap to choose another file',
                  style: TextStyle(
                    fontSize: 12,
                    color: KarryamColors.tealInk.forInk(context),
                  ),
                ),
              ),
          ],
        ),
      ),
    ),
  );
}

Color setupStateColor(SetupState state) => switch (state) {
  SetupState.complete => KarryamColors.tealInk,
  SetupState.underReview => const Color(0xFF2457A6),
  SetupState.attention => KarryamColors.error,
  SetupState.inProgress => const Color(0xFF986000),
  SetupState.notStarted => const Color(0xFF677387),
};

Duration motionDuration(BuildContext context) =>
    MediaQuery.disableAnimationsOf(context)
    ? Duration.zero
    : const Duration(milliseconds: 280);

PageRoute<T> setupRoute<T>(Widget screen) => PageRouteBuilder<T>(
  pageBuilder: (context, animation, secondaryAnimation) => screen,
  transitionDuration: const Duration(milliseconds: 300),
  reverseTransitionDuration: const Duration(milliseconds: 240),
  transitionsBuilder: (context, animation, secondaryAnimation, child) {
    if (MediaQuery.disableAnimationsOf(context)) return child;
    final eased = CurvedAnimation(
      parent: animation,
      curve: Curves.easeOutCubic,
      reverseCurve: Curves.easeInCubic,
    );
    return FadeTransition(
      opacity: eased,
      child: SlideTransition(
        position: Tween(
          begin: const Offset(.06, 0),
          end: Offset.zero,
        ).animate(eased),
        child: child,
      ),
    );
  },
);

Color verificationColor(Object? status) => setupStateColor(
  needsAttention(status)
      ? SetupState.attention
      : reviewPending(status)
      ? SetupState.underReview
      : ['VERIFIED', 'SATISFIED', 'ACTIVATED'].contains(status)
      ? SetupState.complete
      : status == 'DRAFT'
      ? SetupState.inProgress
      : SetupState.notStarted,
);

class SetupStepIndicator extends StatelessWidget {
  const SetupStepIndicator({
    super.key,
    required this.current,
    required this.completed,
  });
  final int current;
  final List<bool> completed;
  static const _labels = [
    'Business details',
    'Authorized representative',
    'Business types',
    'Brands and branches',
    'Bank details',
    'Documents and verification',
  ];
  @override
  Widget build(BuildContext context) => Row(
    children: [
      for (var i = 0; i < 6; i++) ...[
        if (i > 0)
          Expanded(
            child: Container(
              height: 2,
              color:
                  (completed[i - 1]
                          ? KarryamColors.teal.withValues(alpha: .5)
                          : const Color(0xFFDDE3DF))
                      .forSurface(context),
            ),
          ),
        Semantics(
          label:
              '${_labels[i]}, ${i == current
                  ? 'current'
                  : completed[i]
                  ? 'complete'
                  : 'incomplete'}',
          child: AnimatedContainer(
            duration: motionDuration(context),
            width: 24,
            height: 24,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color:
                  (i == current
                          ? KarryamColors.navy
                          : completed[i]
                          ? KarryamColors.teal
                          : const Color(0xFFE6EBE7))
                      .forSurface(context),
              border: i == current
                  ? Border.all(
                      color: KarryamColors.teal.forOutline(context),
                      width: 2,
                    )
                  : null,
            ),
            child: Center(
              child: completed[i] && i != current
                  ? Icon(
                      Icons.check_rounded,
                      size: 14,
                      color: KarryamColors.navy.forInk(context),
                    )
                  : Container(
                      width: i == current ? 7 : 4,
                      height: i == current ? 7 : 4,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color:
                            (i == current
                                    ? KarryamColors.teal
                                    : const Color(0xFF9BA79F))
                                .forSurface(context),
                      ),
                    ),
            ),
          ),
        ),
      ],
    ],
  );
}
