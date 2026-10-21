import 'package:flutter/material.dart';

import '../../design_system/karryam_design_system.dart';

enum KarryamAuthEdgeState {
  accountNotFound,
  duplicateSignup,
  resendCooldown,
  resendSuccess,
  verificationLocked,
  sessionExpired,
}

class KarryamAuthEdgePreview extends StatelessWidget {
  const KarryamAuthEdgePreview({super.key, required this.state});

  final KarryamAuthEdgeState state;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: KarryamColors.backgroundSubtle,
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 430),
            child: Padding(
              padding: const EdgeInsets.all(KarryamSpacing.lg),
              child: _content(context),
            ),
          ),
        ),
      ),
    );
  }

  Widget _content(BuildContext context) {
    return switch (state) {
      KarryamAuthEdgeState.accountNotFound => _StateLayout(
          icon: Icons.person_search_rounded,
          title: 'We couldn’t find that account',
          message: 'No provider account exists for this phone number or email. Check the details or create a new provider account.',
          noticeTitle: 'Account not found',
          noticeMessage: 'Login returns a not-found response when the identifier is not registered.',
          noticeTone: KarryamNoticeTone.warning,
          primaryLabel: 'Check details',
          secondaryLabel: 'Create provider account',
        ),
      KarryamAuthEdgeState.duplicateSignup => _StateLayout(
          icon: Icons.manage_accounts_rounded,
          title: 'Account already exists',
          message: 'This phone number or email is already associated with a Karryam provider account.',
          noticeTitle: 'Use secure sign in',
          noticeMessage: 'Don’t create a duplicate provider profile. Return to login and request a fresh OTP.',
          noticeTone: KarryamNoticeTone.info,
          primaryLabel: 'Sign in instead',
          secondaryLabel: 'Use different details',
        ),
      KarryamAuthEdgeState.resendCooldown => _OtpStateLayout(
          title: 'A new code isn’t ready yet',
          message: 'For your security, wait before requesting another verification code.',
          countdown: '00:32',
          noticeTitle: 'Resend protection',
          noticeMessage: 'The current OTP remains valid until it expires or a fresh code is issued.',
          noticeTone: KarryamNoticeTone.info,
          primaryLabel: 'Verify current code',
          secondaryLabel: 'Resend available in 00:32',
        ),
      KarryamAuthEdgeState.resendSuccess => _OtpStateLayout(
          title: 'Fresh code sent',
          message: 'We sent a new verification code to +234 801 ••• 5678.',
          countdown: '04:59',
          noticeTitle: 'Use the newest code',
          noticeMessage: 'Older codes may no longer be accepted after a resend.',
          noticeTone: KarryamNoticeTone.success,
          primaryLabel: 'Enter new code',
          secondaryLabel: 'Resend later',
        ),
      KarryamAuthEdgeState.verificationLocked => _StateLayout(
          icon: Icons.lock_clock_rounded,
          title: 'Verification temporarily locked',
          message: 'Too many unsuccessful attempts were made. Wait for the verification window to reset before trying again.',
          noticeTitle: 'Security protection active',
          noticeMessage: 'Karryam never bypasses OTP verification after repeated failed attempts.',
          noticeTone: KarryamNoticeTone.danger,
          primaryLabel: 'Return to sign in',
        ),
      KarryamAuthEdgeState.sessionExpired => _StateLayout(
          icon: Icons.history_toggle_off_rounded,
          title: 'Your session has expired',
          message: 'Your secure session can no longer be refreshed. Sign in again to continue provider operations.',
          noticeTitle: 'Your work is protected',
          noticeMessage: 'Karryam will reload the latest authoritative mission state after you sign in again.',
          noticeTone: KarryamNoticeTone.warning,
          primaryLabel: 'Sign in again',
        ),
    };
  }
}

class _StateLayout extends StatelessWidget {
  const _StateLayout({
    required this.icon,
    required this.title,
    required this.message,
    required this.noticeTitle,
    required this.noticeMessage,
    required this.noticeTone,
    required this.primaryLabel,
    this.secondaryLabel,
  });

  final IconData icon;
  final String title;
  final String message;
  final String noticeTitle;
  final String noticeMessage;
  final KarryamNoticeTone noticeTone;
  final String primaryLabel;
  final String? secondaryLabel;

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        Container(
          width: 92,
          height: 92,
          decoration: const BoxDecoration(color: KarryamColors.teal10, shape: BoxShape.circle),
          alignment: Alignment.center,
          child: Icon(icon, size: 42, color: KarryamColors.midnightNavy),
        ),
        const SizedBox(height: KarryamSpacing.xl),
        Text(title, textAlign: TextAlign.center, style: Theme.of(context).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w900)),
        const SizedBox(height: KarryamSpacing.sm),
        Text(message, textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray, height: 1.45)),
        const SizedBox(height: KarryamSpacing.xl),
        KarryamNotice(title: noticeTitle, message: noticeMessage, tone: noticeTone),
        const SizedBox(height: KarryamSpacing.xl),
        KarryamButton(label: primaryLabel, onPressed: () {}),
        if (secondaryLabel != null) ...[
          const SizedBox(height: KarryamSpacing.sm),
          KarryamButton(label: secondaryLabel!, variant: KarryamButtonVariant.secondary, onPressed: () {}),
        ],
      ],
    );
  }
}

class _OtpStateLayout extends StatelessWidget {
  const _OtpStateLayout({
    required this.title,
    required this.message,
    required this.countdown,
    required this.noticeTitle,
    required this.noticeMessage,
    required this.noticeTone,
    required this.primaryLabel,
    required this.secondaryLabel,
  });

  final String title;
  final String message;
  final String countdown;
  final String noticeTitle;
  final String noticeMessage;
  final KarryamNoticeTone noticeTone;
  final String primaryLabel;
  final String secondaryLabel;

  @override
  Widget build(BuildContext context) {
    return ListView(
      padding: const EdgeInsets.only(top: 36, bottom: 24),
      children: [
        Text('SECURE VERIFICATION', style: Theme.of(context).textTheme.labelSmall?.copyWith(color: KarryamColors.auroraTeal, fontWeight: FontWeight.w800, letterSpacing: 1.3)),
        const SizedBox(height: KarryamSpacing.sm),
        Text(title, style: Theme.of(context).textTheme.headlineLarge?.copyWith(fontWeight: FontWeight.w900)),
        const SizedBox(height: KarryamSpacing.xs),
        Text(message, style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
        const SizedBox(height: KarryamSpacing.xl),
        Row(
          children: List.generate(6, (index) => Expanded(
            child: Container(
              height: 58,
              margin: EdgeInsets.only(right: index == 5 ? 0 : 8),
              decoration: BoxDecoration(color: KarryamColors.pureWhite, borderRadius: BorderRadius.circular(KarryamRadius.md), border: Border.all(color: KarryamColors.border)),
            ),
          )),
        ),
        const SizedBox(height: KarryamSpacing.md),
        Row(
          children: [
            const Icon(Icons.timer_outlined, size: 18, color: KarryamColors.navy60),
            const SizedBox(width: 6),
            Text('Code window $countdown', style: Theme.of(context).textTheme.bodyMedium?.copyWith(color: KarryamColors.navy60, fontWeight: FontWeight.w700)),
          ],
        ),
        const SizedBox(height: KarryamSpacing.lg),
        KarryamNotice(title: noticeTitle, message: noticeMessage, tone: noticeTone),
        const SizedBox(height: KarryamSpacing.xl),
        KarryamButton(label: primaryLabel, onPressed: () {}),
        const SizedBox(height: KarryamSpacing.sm),
        KarryamButton(label: secondaryLabel, variant: KarryamButtonVariant.secondary, onPressed: () {}),
      ],
    );
  }
}
