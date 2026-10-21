import 'package:flutter/material.dart';

import '../../design_system/karryam_design_system.dart';
import '../../design_system/components/karryam_verification_step_card.dart';

enum KarryamVerificationPreviewState {
  hub,
  identityStart,
  identitySubmitted,
  faceGuide,
  faceProcessing,
  faceFailed,
  licence,
  licencePending,
  licenceRejected,
  verified,
}

class KarryamVerificationPreview extends StatelessWidget {
  const KarryamVerificationPreview({super.key, required this.state});

  final KarryamVerificationPreviewState state;

  @override
  Widget build(BuildContext context) {
    return switch (state) {
      KarryamVerificationPreviewState.hub => const _Hub(),
      KarryamVerificationPreviewState.identityStart => const _IdentityStart(),
      KarryamVerificationPreviewState.identitySubmitted => const _IdentitySubmitted(),
      KarryamVerificationPreviewState.faceGuide => const _FaceGuide(),
      KarryamVerificationPreviewState.faceProcessing => const _FaceProcessing(),
      KarryamVerificationPreviewState.faceFailed => const _FaceFailed(),
      KarryamVerificationPreviewState.licence => const _Licence(),
      KarryamVerificationPreviewState.licencePending => const _LicencePending(),
      KarryamVerificationPreviewState.licenceRejected => const _LicenceRejected(),
      KarryamVerificationPreviewState.verified => const _Verified(),
    };
  }
}

class _Frame extends StatelessWidget {
  const _Frame({required this.title, required this.child});
  final String title;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: KarryamColors.backgroundSubtle,
      appBar: AppBar(
        leading: IconButton(onPressed: () {}, icon: const Icon(Icons.arrow_back_rounded)),
        title: Text(title),
      ),
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 430),
            child: child,
          ),
        ),
      ),
    );
  }
}

class _Hub extends StatelessWidget {
  const _Hub();
  @override
  Widget build(BuildContext context) {
    return _Frame(
      title: 'Readiness',
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 20, 24, 28),
        children: [
          Text('You’re 67% ready', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: 8),
          Text('Complete the remaining requirements before going online.', style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 20),
          ClipRRect(
            borderRadius: BorderRadius.circular(KarryamRadius.pill),
            child: const LinearProgressIndicator(value: .67, minHeight: 7, color: KarryamColors.auroraTeal, backgroundColor: KarryamColors.navy10),
          ),
          const SizedBox(height: 28),
          KarryamVerificationStepCard(title: 'Identity', subtitle: 'Government ID and legal details', icon: Icons.badge_outlined, state: KarryamVerificationStepState.approved, onTap: () {}),
          const SizedBox(height: 12),
          KarryamVerificationStepCard(title: 'Face verification', subtitle: 'Live selfie and identity match', icon: Icons.face_retouching_natural_outlined, state: KarryamVerificationStepState.submitted, onTap: () {}),
          const SizedBox(height: 12),
          KarryamVerificationStepCard(title: 'Driver licence', subtitle: 'Licence number, expiry and document', icon: Icons.contact_page_outlined, state: KarryamVerificationStepState.rejected, onTap: () {}),
          const SizedBox(height: 12),
          KarryamVerificationStepCard(title: 'Vehicle', subtitle: 'Vehicle and document review', icon: Icons.two_wheeler_rounded, state: KarryamVerificationStepState.submitted, onTap: () {}),
          const SizedBox(height: 12),
          KarryamVerificationStepCard(title: 'Guarantor', subtitle: 'Provider safety requirement', icon: Icons.handshake_outlined, state: KarryamVerificationStepState.approved, onTap: () {}),
          const SizedBox(height: 12),
          KarryamVerificationStepCard(title: 'Emergency contact', subtitle: 'Safety contact information', icon: Icons.health_and_safety_outlined, state: KarryamVerificationStepState.approved, onTap: () {}),
        ],
      ),
    );
  }
}

class _IdentityStart extends StatelessWidget {
  const _IdentityStart();
  @override
  Widget build(BuildContext context) {
    return _Frame(
      title: 'Identity verification',
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 24, 24, 28),
        children: [
          Text('Verify your identity', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: 8),
          Text('Use a valid government-issued ID that matches the legal name on your provider profile.', style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 24),
          const KarryamTextField(label: 'Government ID type', hint: 'National ID', leading: Icon(Icons.badge_outlined), trailing: Icon(Icons.expand_more_rounded)),
          const SizedBox(height: 16),
          const KarryamTextField(label: 'ID number', hint: 'Enter ID number', leading: Icon(Icons.numbers_rounded)),
          const SizedBox(height: 20),
          Container(
            padding: const EdgeInsets.all(20),
            decoration: BoxDecoration(color: KarryamColors.pureWhite, borderRadius: BorderRadius.circular(KarryamRadius.lg), border: Border.all(color: KarryamColors.border)),
            child: Column(children: [
              const Icon(Icons.upload_file_rounded, size: 34, color: KarryamColors.midnightNavy),
              const SizedBox(height: 10),
              Text('Upload government ID', style: Theme.of(context).textTheme.titleMedium),
              const SizedBox(height: 6),
              Text('JPEG, PNG or PDF', style: Theme.of(context).textTheme.bodyMedium),
              const SizedBox(height: 14),
              KarryamButton(label: 'Choose document', variant: KarryamButtonVariant.secondary, onPressed: () {}),
            ]),
          ),
          const SizedBox(height: 24),
          KarryamButton(label: 'Submit identity', onPressed: () {}),
        ],
      ),
    );
  }
}

class _IdentitySubmitted extends StatelessWidget {
  const _IdentitySubmitted();
  @override
  Widget build(BuildContext context) {
    return _Frame(
      title: 'Identity verification',
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const _StateBadge(icon: Icons.check_rounded, background: KarryamColors.teal10, foreground: KarryamColors.success),
            const SizedBox(height: 24),
            Text('Identity submitted', style: Theme.of(context).textTheme.headlineLarge),
            const SizedBox(height: 10),
            Text('Karryam is reviewing your government ID and profile details.', textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
            const SizedBox(height: 24),
            const KarryamNotice(title: 'No action needed', message: 'You can continue with the remaining readiness requirements while this review is in progress.', tone: KarryamNoticeTone.success),
            const SizedBox(height: 24),
            KarryamButton(label: 'Back to readiness', onPressed: () {}),
          ],
        ),
      ),
    );
  }
}

class _FaceGuide extends StatelessWidget {
  const _FaceGuide();
  @override
  Widget build(BuildContext context) {
    return _Frame(
      title: 'Face verification',
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 24, 24, 28),
        children: [
          Text('Confirm it’s really you', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: 8),
          Text('We compare a live selfie against your identity profile.', style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 24),
          Container(
            height: 250,
            decoration: BoxDecoration(color: KarryamColors.navy10, borderRadius: BorderRadius.circular(KarryamRadius.xl)),
            alignment: Alignment.center,
            child: Stack(alignment: Alignment.center, children: [
              Container(width: 150, height: 190, decoration: BoxDecoration(border: Border.all(color: KarryamColors.auroraTeal, width: 2), borderRadius: BorderRadius.circular(80))),
              const Icon(Icons.face_rounded, size: 96, color: KarryamColors.navy40),
            ]),
          ),
          const SizedBox(height: 20),
          const KarryamNotice(title: 'Before you start', message: 'Use good lighting, remove face coverings and keep only one face visible.'),
          const SizedBox(height: 24),
          KarryamButton(label: 'Start face check', leading: const Icon(Icons.camera_alt_outlined, size: 20), onPressed: () {}),
        ],
      ),
    );
  }
}

class _FaceProcessing extends StatelessWidget {
  const _FaceProcessing();
  @override
  Widget build(BuildContext context) {
    return _Frame(
      title: 'Face verification',
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(mainAxisAlignment: MainAxisAlignment.center, children: [
          const SizedBox.square(dimension: 58, child: CircularProgressIndicator(strokeWidth: 4, color: KarryamColors.auroraTeal)),
          const SizedBox(height: 26),
          Text('Checking your selfie', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: 10),
          Text('Keep this screen open while Karryam securely processes the match.', textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
        ]),
      ),
    );
  }
}

class _FaceFailed extends StatelessWidget {
  const _FaceFailed();
  @override
  Widget build(BuildContext context) {
    return _Frame(
      title: 'Face verification',
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(mainAxisAlignment: MainAxisAlignment.center, children: [
          const _StateBadge(icon: Icons.face_retouching_off_outlined, background: KarryamColors.dangerSurface, foreground: KarryamColors.danger),
          const SizedBox(height: 24),
          Text('We couldn’t confirm the match', style: Theme.of(context).textTheme.headlineLarge, textAlign: TextAlign.center),
          const SizedBox(height: 10),
          Text('Try again in brighter light with your full face clearly visible.', textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 20),
          const KarryamNotice(title: 'Your account is not rejected', message: 'A failed capture only means this attempt could not be verified.', tone: KarryamNoticeTone.warning),
          const SizedBox(height: 24),
          KarryamButton(label: 'Try face check again', onPressed: () {}),
        ]),
      ),
    );
  }
}

class _Licence extends StatelessWidget {
  const _Licence();
  @override
  Widget build(BuildContext context) {
    return _Frame(
      title: 'Driver licence',
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 24, 24, 28),
        children: [
          Text('Add your driver licence', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: 8),
          Text('Karryam uses the licence number, expiry date and document to confirm driving eligibility.', style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 24),
          const KarryamTextField(label: 'Licence number', hint: 'Enter licence number', leading: Icon(Icons.contact_page_outlined)),
          const SizedBox(height: 16),
          const KarryamTextField(label: 'Expiry date', hint: 'MM / YYYY', leading: Icon(Icons.calendar_month_outlined)),
          const SizedBox(height: 20),
          const KarryamNotice(title: 'Document required', message: 'Upload a clear copy of the licence showing the number and expiry date.'),
          const SizedBox(height: 20),
          KarryamButton(label: 'Upload licence', variant: KarryamButtonVariant.secondary, leading: const Icon(Icons.upload_file_rounded, size: 20), onPressed: () {}),
          const SizedBox(height: 12),
          KarryamButton(label: 'Submit for review', onPressed: () {}),
        ],
      ),
    );
  }
}

class _LicencePending extends StatelessWidget {
  const _LicencePending();
  @override
  Widget build(BuildContext context) {
    return _Frame(
      title: 'Driver licence',
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(mainAxisAlignment: MainAxisAlignment.center, children: [
          const _StateBadge(icon: Icons.hourglass_top_rounded, background: KarryamColors.warningSurface, foreground: KarryamColors.warning),
          const SizedBox(height: 24),
          Text('Licence under review', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: 10),
          Text('You can keep completing other requirements while this check is pending.', textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 24),
          KarryamButton(label: 'Back to readiness', onPressed: () {}),
        ]),
      ),
    );
  }
}

class _LicenceRejected extends StatelessWidget {
  const _LicenceRejected();
  @override
  Widget build(BuildContext context) {
    return _Frame(
      title: 'Driver licence',
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 32, 24, 28),
        children: [
          const _StateBadge(icon: Icons.priority_high_rounded, background: KarryamColors.dangerSurface, foreground: KarryamColors.danger),
          const SizedBox(height: 24),
          Text('Licence needs attention', style: Theme.of(context).textTheme.headlineLarge, textAlign: TextAlign.center),
          const SizedBox(height: 10),
          Text('The uploaded document could not be approved.', textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 22),
          const KarryamNotice(title: 'Reason from review', message: 'The expiry date on the document is not readable. Upload a clearer copy.', tone: KarryamNoticeTone.danger),
          const SizedBox(height: 24),
          KarryamButton(label: 'Replace licence document', onPressed: () {}),
          const SizedBox(height: 12),
          KarryamButton(label: 'Back to readiness', variant: KarryamButtonVariant.secondary, onPressed: () {}),
        ],
      ),
    );
  }
}

class _Verified extends StatelessWidget {
  const _Verified();
  @override
  Widget build(BuildContext context) {
    return _Frame(
      title: 'Readiness',
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(mainAxisAlignment: MainAxisAlignment.center, children: [
          const _StateBadge(icon: Icons.verified_rounded, background: KarryamColors.teal10, foreground: KarryamColors.success),
          const SizedBox(height: 24),
          Text('You’re verified', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: 10),
          Text('All required provider verification checks are approved.', textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 22),
          const KarryamNotice(title: 'One more operational check', message: 'A verified account still needs an approved vehicle and valid location readiness before going online.', tone: KarryamNoticeTone.success),
          const SizedBox(height: 24),
          KarryamButton(label: 'Continue to vehicle', variant: KarryamButtonVariant.accent, onPressed: () {}),
        ]),
      ),
    );
  }
}

class _StateBadge extends StatelessWidget {
  const _StateBadge({required this.icon, required this.background, required this.foreground});
  final IconData icon;
  final Color background;
  final Color foreground;
  @override
  Widget build(BuildContext context) => Container(width: 92, height: 92, decoration: BoxDecoration(color: background, shape: BoxShape.circle), alignment: Alignment.center, child: Icon(icon, color: foreground, size: 42));
}
