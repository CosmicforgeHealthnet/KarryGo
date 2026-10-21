import 'package:flutter/material.dart';

import '../../design_system/karryam_design_system.dart';

enum KarryamAuthPreviewState {
  splash,
  restoringSession,
  login,
  loginSubmitting,
  signup,
  signupValidation,
  otp,
  otpInvalid,
  otpExpired,
  noNetwork,
  suspended,
}

class KarryamAuthPreview extends StatelessWidget {
  const KarryamAuthPreview({
    super.key,
    required this.state,
  });

  final KarryamAuthPreviewState state;

  @override
  Widget build(BuildContext context) {
    return switch (state) {
      KarryamAuthPreviewState.splash => const _SplashPreview(),
      KarryamAuthPreviewState.restoringSession => const _RestoringPreview(),
      KarryamAuthPreviewState.login => const _LoginPreview(),
      KarryamAuthPreviewState.loginSubmitting => const _LoginPreview(isSubmitting: true),
      KarryamAuthPreviewState.signup => const _SignupPreview(),
      KarryamAuthPreviewState.signupValidation => const _SignupPreview(showErrors: true),
      KarryamAuthPreviewState.otp => const _OtpPreview(),
      KarryamAuthPreviewState.otpInvalid => const _OtpPreview(invalid: true),
      KarryamAuthPreviewState.otpExpired => const _OtpPreview(expired: true),
      KarryamAuthPreviewState.noNetwork => const _NoNetworkPreview(),
      KarryamAuthPreviewState.suspended => const _SuspendedPreview(),
    };
  }
}

class _AuthScaffold extends StatelessWidget {
  const _AuthScaffold({
    required this.child,
    this.showBack = false,
  });

  final Widget child;
  final bool showBack;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: KarryamColors.backgroundSubtle,
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 430),
            child: Column(
              children: [
                SizedBox(
                  height: 64,
                  child: Padding(
                    padding: const EdgeInsets.symmetric(horizontal: KarryamSpacing.lg),
                    child: Row(
                      children: [
                        if (showBack)
                          IconButton(
                            onPressed: () {},
                            tooltip: 'Back',
                            icon: const Icon(Icons.arrow_back_rounded),
                          )
                        else
                          const SizedBox(width: 48),
                        const Spacer(),
                        const _Wordmark(compact: true),
                        const Spacer(),
                        const SizedBox(width: 48),
                      ],
                    ),
                  ),
                ),
                Expanded(child: child),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _Wordmark extends StatelessWidget {
  const _Wordmark({this.compact = false});

  final bool compact;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          width: compact ? 28 : 40,
          height: compact ? 28 : 40,
          decoration: BoxDecoration(
            color: KarryamColors.midnightNavy,
            borderRadius: BorderRadius.circular(compact ? 9 : 12),
          ),
          alignment: Alignment.center,
          child: Text(
            'K',
            style: TextStyle(
              color: KarryamColors.pureWhite,
              fontSize: compact ? 14 : 20,
              fontWeight: FontWeight.w900,
            ),
          ),
        ),
        SizedBox(width: compact ? 8 : 10),
        Text(
          'Karryam',
          style: Theme.of(context).textTheme.titleMedium?.copyWith(
                color: KarryamColors.midnightNavy,
                fontWeight: FontWeight.w900,
                letterSpacing: -0.3,
              ),
        ),
      ],
    );
  }
}

class _SplashPreview extends StatelessWidget {
  const _SplashPreview();

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Container(
        decoration: const BoxDecoration(gradient: KarryamColors.motionGradient),
        child: SafeArea(
          child: Padding(
            padding: const EdgeInsets.all(KarryamSpacing.xl),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Spacer(),
                const _Wordmark(),
                const SizedBox(height: KarryamSpacing.xl),
                Text(
                  'Move with confidence.',
                  style: Theme.of(context).textTheme.displaySmall?.copyWith(
                        color: KarryamColors.pureWhite,
                        fontWeight: FontWeight.w900,
                        height: 1.02,
                      ),
                ),
                const SizedBox(height: KarryamSpacing.sm),
                Text(
                  'Operate with intelligence.',
                  style: Theme.of(context).textTheme.headlineMedium?.copyWith(
                        color: KarryamColors.teal20,
                        fontWeight: FontWeight.w800,
                      ),
                ),
                const SizedBox(height: KarryamSpacing.lg),
                Text(
                  'Provider operations built for fast decisions, secure custody and reliable delivery.',
                  style: Theme.of(context).textTheme.bodyLarge?.copyWith(
                        color: KarryamColors.pureWhite.withValues(alpha: .82),
                        height: 1.45,
                      ),
                ),
                const Spacer(),
                const LinearProgressIndicator(
                  minHeight: 3,
                  backgroundColor: Color(0x33FFFFFF),
                  color: KarryamColors.auroraTeal,
                ),
                const SizedBox(height: KarryamSpacing.md),
                Text(
                  'KARRYAM • DISPATCH PROVIDER',
                  style: Theme.of(context).textTheme.labelSmall?.copyWith(
                        color: KarryamColors.pureWhite.withValues(alpha: .72),
                        letterSpacing: 1.6,
                        fontWeight: FontWeight.w700,
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

class _RestoringPreview extends StatelessWidget {
  const _RestoringPreview();

  @override
  Widget build(BuildContext context) {
    return _AuthScaffold(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: KarryamSpacing.lg),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Container(
              width: 88,
              height: 88,
              decoration: const BoxDecoration(
                color: KarryamColors.teal10,
                shape: BoxShape.circle,
              ),
              alignment: Alignment.center,
              child: const SizedBox.square(
                dimension: 34,
                child: CircularProgressIndicator(
                  strokeWidth: 3,
                  color: KarryamColors.auroraTeal,
                ),
              ),
            ),
            const SizedBox(height: KarryamSpacing.xl),
            Text('Restoring your workspace', style: Theme.of(context).textTheme.headlineSmall),
            const SizedBox(height: KarryamSpacing.sm),
            Text(
              'Checking your secure session and active delivery state.',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray),
            ),
          ],
        ),
      ),
    );
  }
}

class _LoginPreview extends StatelessWidget {
  const _LoginPreview({this.isSubmitting = false});

  final bool isSubmitting;

  @override
  Widget build(BuildContext context) {
    return _AuthScaffold(
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 44, 24, 32),
        children: [
          _Eyebrow(label: 'PROVIDER ACCESS'),
          const SizedBox(height: KarryamSpacing.sm),
          Text('Welcome back', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: KarryamSpacing.xs),
          Text(
            'Use your registered phone number or email. We’ll send a one-time code to confirm it’s you.',
            style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray),
          ),
          const SizedBox(height: KarryamSpacing.xl),
          const KarryamTextField(
            label: 'Phone number or email',
            hint: '+234 801 234 5678',
            keyboardType: TextInputType.emailAddress,
            leading: Icon(Icons.person_outline_rounded),
          ),
          const SizedBox(height: KarryamSpacing.lg),
          KarryamButton(
            label: isSubmitting ? 'Sending code' : 'Continue securely',
            isLoading: isSubmitting,
            onPressed: isSubmitting ? null : () {},
          ),
          const SizedBox(height: KarryamSpacing.lg),
          Row(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Text('New to Karryam?', style: Theme.of(context).textTheme.bodyMedium),
              TextButton(onPressed: () {}, child: const Text('Create provider account')),
            ],
          ),
          const SizedBox(height: KarryamSpacing.xl),
          const _TrustStrip(),
        ],
      ),
    );
  }
}

class _SignupPreview extends StatelessWidget {
  const _SignupPreview({this.showErrors = false});

  final bool showErrors;

  @override
  Widget build(BuildContext context) {
    return _AuthScaffold(
      showBack: true,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 32, 24, 32),
        children: [
          _Eyebrow(label: 'CREATE ACCOUNT'),
          const SizedBox(height: KarryamSpacing.sm),
          Text('Start your provider journey', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: KarryamSpacing.xs),
          Text(
            'We use your phone for secure access and your email for account notices.',
            style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray),
          ),
          const SizedBox(height: KarryamSpacing.xl),
          KarryamTextField(
            label: 'Phone number',
            hint: '+234 801 234 5678',
            keyboardType: TextInputType.phone,
            leading: const Icon(Icons.phone_outlined),
            errorText: showErrors ? 'Enter a valid Nigerian mobile number.' : null,
          ),
          const SizedBox(height: KarryamSpacing.lg),
          KarryamTextField(
            label: 'Email address',
            hint: 'you@example.com',
            keyboardType: TextInputType.emailAddress,
            leading: const Icon(Icons.mail_outline_rounded),
            errorText: showErrors ? 'Enter a valid email address.' : null,
          ),
          if (showErrors) ...[
            const SizedBox(height: KarryamSpacing.md),
            const KarryamNotice(
              title: 'Check your details',
              message: 'We need a valid phone number and email before we can send your verification code.',
              tone: KarryamNoticeTone.danger,
            ),
          ],
          const SizedBox(height: KarryamSpacing.xl),
          KarryamButton(label: 'Send verification code', onPressed: () {}),
          const SizedBox(height: KarryamSpacing.md),
          Text(
            'By continuing, you agree to Karryam’s provider terms and privacy notice.',
            textAlign: TextAlign.center,
            style: Theme.of(context).textTheme.bodySmall?.copyWith(color: KarryamColors.navy60),
          ),
        ],
      ),
    );
  }
}

class _OtpPreview extends StatelessWidget {
  const _OtpPreview({this.invalid = false, this.expired = false});

  final bool invalid;
  final bool expired;

  @override
  Widget build(BuildContext context) {
    return _AuthScaffold(
      showBack: true,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 32, 24, 32),
        children: [
          _Eyebrow(label: 'SECURE VERIFICATION'),
          const SizedBox(height: KarryamSpacing.sm),
          Text('Enter your 6-digit code', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: KarryamSpacing.xs),
          Text(
            'We sent a one-time code to +234 801 ••• 5678.',
            style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray),
          ),
          const SizedBox(height: KarryamSpacing.xl),
          Row(
            children: List.generate(6, (index) {
              final value = invalid ? [4, 1, 8, 2, 9, 3][index].toString() : index < 3 ? '${index + 2}' : '';
              return Expanded(
                child: Container(
                  height: 58,
                  margin: EdgeInsets.only(right: index == 5 ? 0 : 8),
                  alignment: Alignment.center,
                  decoration: BoxDecoration(
                    color: KarryamColors.pureWhite,
                    borderRadius: BorderRadius.circular(KarryamRadius.md),
                    border: Border.all(
                      color: invalid ? KarryamColors.danger : index == 3 ? KarryamColors.auroraTeal : KarryamColors.border,
                      width: index == 3 ? 1.5 : 1,
                    ),
                  ),
                  child: Text(value, style: Theme.of(context).textTheme.titleLarge),
                ),
              );
            }),
          ),
          if (invalid) ...[
            const SizedBox(height: KarryamSpacing.md),
            const KarryamNotice(
              title: 'That code is incorrect',
              message: 'Check the code and try again. Repeated incorrect attempts may temporarily lock verification.',
              tone: KarryamNoticeTone.danger,
            ),
          ],
          if (expired) ...[
            const SizedBox(height: KarryamSpacing.md),
            const KarryamNotice(
              title: 'Code expired',
              message: 'Request a fresh code to continue securely.',
              tone: KarryamNoticeTone.warning,
            ),
          ],
          const SizedBox(height: KarryamSpacing.xl),
          KarryamButton(
            label: expired ? 'Request new code' : 'Verify and continue',
            variant: expired ? KarryamButtonVariant.accent : KarryamButtonVariant.primary,
            onPressed: () {},
          ),
          const SizedBox(height: KarryamSpacing.sm),
          TextButton(
            onPressed: expired ? null : () {},
            child: Text(expired ? 'Previous code is no longer valid' : 'Resend code in 00:32'),
          ),
          const SizedBox(height: KarryamSpacing.xl),
          const _TrustStrip(),
        ],
      ),
    );
  }
}

class _NoNetworkPreview extends StatelessWidget {
  const _NoNetworkPreview();

  @override
  Widget build(BuildContext context) {
    return _AuthScaffold(
      child: Padding(
        padding: const EdgeInsets.all(KarryamSpacing.lg),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const _StateIcon(icon: Icons.wifi_off_rounded, tone: KarryamNoticeTone.warning),
            const SizedBox(height: KarryamSpacing.xl),
            Text('You’re offline', style: Theme.of(context).textTheme.headlineLarge),
            const SizedBox(height: KarryamSpacing.sm),
            Text(
              'Karryam needs a connection to start or restore your secure provider session.',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray),
            ),
            const SizedBox(height: KarryamSpacing.xl),
            KarryamButton(
              label: 'Try again',
              leading: const Icon(Icons.refresh_rounded, size: 20),
              onPressed: () {},
            ),
            const SizedBox(height: KarryamSpacing.sm),
            KarryamButton(
              label: 'Open connection settings',
              variant: KarryamButtonVariant.secondary,
              onPressed: () {},
            ),
          ],
        ),
      ),
    );
  }
}

class _SuspendedPreview extends StatelessWidget {
  const _SuspendedPreview();

  @override
  Widget build(BuildContext context) {
    return _AuthScaffold(
      child: Padding(
        padding: const EdgeInsets.all(KarryamSpacing.lg),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const _StateIcon(icon: Icons.shield_outlined, tone: KarryamNoticeTone.danger),
            const SizedBox(height: KarryamSpacing.xl),
            Text('Account access restricted', style: Theme.of(context).textTheme.headlineLarge, textAlign: TextAlign.center),
            const SizedBox(height: KarryamSpacing.sm),
            Text(
              'Your provider account currently cannot access operations. Review the account notice or contact Karryam support.',
              textAlign: TextAlign.center,
              style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray),
            ),
            const SizedBox(height: KarryamSpacing.lg),
            const KarryamNotice(
              title: 'Reference: KA-ACCESS-2048',
              message: 'Keep this reference when contacting support. Sensitive customer or delivery data is not shown here.',
              tone: KarryamNoticeTone.danger,
            ),
            const SizedBox(height: KarryamSpacing.xl),
            KarryamButton(label: 'Contact support', onPressed: () {}),
            const SizedBox(height: KarryamSpacing.sm),
            KarryamButton(
              label: 'Sign out',
              variant: KarryamButtonVariant.secondary,
              onPressed: () {},
            ),
          ],
        ),
      ),
    );
  }
}

class _Eyebrow extends StatelessWidget {
  const _Eyebrow({required this.label});

  final String label;

  @override
  Widget build(BuildContext context) {
    return Text(
      label,
      style: Theme.of(context).textTheme.labelSmall?.copyWith(
            color: KarryamColors.auroraTeal,
            fontWeight: FontWeight.w900,
            letterSpacing: 1.4,
          ),
    );
  }
}

class _TrustStrip extends StatelessWidget {
  const _TrustStrip();

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(KarryamSpacing.md),
      decoration: BoxDecoration(
        color: KarryamColors.navy10,
        borderRadius: BorderRadius.circular(KarryamRadius.md),
      ),
      child: Row(
        children: [
          const Icon(Icons.lock_outline_rounded, color: KarryamColors.midnightNavy, size: 20),
          const SizedBox(width: KarryamSpacing.sm),
          Expanded(
            child: Text(
              'Passwordless access • One-time verification • Secure session',
              style: Theme.of(context).textTheme.bodySmall?.copyWith(
                    color: KarryamColors.indigoGray,
                    fontWeight: FontWeight.w600,
                  ),
            ),
          ),
        ],
      ),
    );
  }
}

class _StateIcon extends StatelessWidget {
  const _StateIcon({required this.icon, required this.tone});

  final IconData icon;
  final KarryamNoticeTone tone;

  @override
  Widget build(BuildContext context) {
    final (background, foreground) = switch (tone) {
      KarryamNoticeTone.info => (KarryamColors.infoSurface, KarryamColors.info),
      KarryamNoticeTone.success => (KarryamColors.successSurface, KarryamColors.success),
      KarryamNoticeTone.warning => (KarryamColors.warningSurface, KarryamColors.warning),
      KarryamNoticeTone.danger => (KarryamColors.dangerSurface, KarryamColors.danger),
    };
    return Container(
      width: 92,
      height: 92,
      decoration: BoxDecoration(color: background, shape: BoxShape.circle),
      alignment: Alignment.center,
      child: Icon(icon, color: foreground, size: 38),
    );
  }
}
