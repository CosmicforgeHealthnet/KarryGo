import 'package:flutter/material.dart';

import '../../design_system/karryam_design_system.dart';

enum KarryamOnboardingPreviewState {
  intro,
  accountType,
  serviceType,
  operationMode,
  profileDetails,
  photoGuidance,
  photoPreview,
  bikeType,
  driverInformation,
  review,
  submitting,
  verificationPending,
}

class KarryamOnboardingPreview extends StatelessWidget {
  const KarryamOnboardingPreview({super.key, required this.state});

  final KarryamOnboardingPreviewState state;

  @override
  Widget build(BuildContext context) {
    return switch (state) {
      KarryamOnboardingPreviewState.intro => const _IntroScreen(),
      KarryamOnboardingPreviewState.accountType => const _ChoiceScreen(
          step: 1,
          title: 'How will you use Karryam?',
          subtitle: 'Choose the account path that matches how you plan to work.',
          options: [
            _OptionData(Icons.person_outline_rounded, 'Individual provider', 'Operate your own delivery account.'),
            _OptionData(Icons.groups_2_outlined, 'Fleet provider', 'Operate as part of a managed provider organization.'),
          ],
        ),
      KarryamOnboardingPreviewState.serviceType => const _ChoiceScreen(
          step: 2,
          title: 'Choose your service',
          subtitle: 'Your service selection controls the setup requirements we show next.',
          options: [
            _OptionData(Icons.local_shipping_outlined, 'Dispatch delivery', 'Small and medium parcel delivery missions.'),
            _OptionData(Icons.inventory_2_outlined, 'Package logistics', 'Structured parcel and custody workflows.'),
          ],
        ),
      KarryamOnboardingPreviewState.operationMode => const _ChoiceScreen(
          step: 3,
          title: 'How do you operate?',
          subtitle: 'This controls whether readiness is managed individually or through a fleet.',
          options: [
            _OptionData(Icons.person_pin_circle_outlined, 'Individual', 'You control availability, vehicle and verification.'),
            _OptionData(Icons.apartment_rounded, 'Fleet-managed', 'Your organization can manage assignment and readiness.'),
          ],
        ),
      KarryamOnboardingPreviewState.profileDetails => const _ProfileDetailsScreen(),
      KarryamOnboardingPreviewState.photoGuidance => const _PhotoGuidanceScreen(),
      KarryamOnboardingPreviewState.photoPreview => const _PhotoPreviewScreen(),
      KarryamOnboardingPreviewState.bikeType => const _BikeTypeScreen(),
      KarryamOnboardingPreviewState.driverInformation => const _DriverInfoScreen(),
      KarryamOnboardingPreviewState.review => const _ReviewScreen(),
      KarryamOnboardingPreviewState.submitting => const _SubmittingScreen(),
      KarryamOnboardingPreviewState.verificationPending => const _PendingScreen(),
    };
  }
}

class _SetupScaffold extends StatelessWidget {
  const _SetupScaffold({required this.child, this.step});

  final Widget child;
  final int? step;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: KarryamColors.backgroundSubtle,
      appBar: AppBar(
        leading: IconButton(onPressed: () {}, icon: const Icon(Icons.arrow_back_rounded)),
        title: const Text('Provider setup'),
        actions: [
          TextButton(onPressed: () {}, child: const Text('Save & exit')),
          const SizedBox(width: 8),
        ],
      ),
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 430),
            child: Column(
              children: [
                if (step != null)
                  Padding(
                    padding: const EdgeInsets.fromLTRB(24, 8, 24, 12),
                    child: _Progress(step: step!),
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

class _Progress extends StatelessWidget {
  const _Progress({required this.step});
  final int step;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            Text('STEP $step OF 6', style: Theme.of(context).textTheme.labelSmall?.copyWith(color: KarryamColors.auroraTeal, fontWeight: FontWeight.w900, letterSpacing: 1.2)),
            const Spacer(),
            Text('${((step / 6) * 100).round()}%', style: Theme.of(context).textTheme.labelMedium?.copyWith(color: KarryamColors.indigoGray)),
          ],
        ),
        const SizedBox(height: 8),
        ClipRRect(
          borderRadius: BorderRadius.circular(KarryamRadius.pill),
          child: LinearProgressIndicator(
            minHeight: 5,
            value: step / 6,
            backgroundColor: KarryamColors.navy10,
            color: KarryamColors.auroraTeal,
          ),
        ),
      ],
    );
  }
}

class _IntroScreen extends StatelessWidget {
  const _IntroScreen();

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: KarryamColors.midnightNavy,
      body: SafeArea(
        child: Padding(
          padding: const EdgeInsets.all(28),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const Spacer(),
              Container(
                width: 72,
                height: 72,
                decoration: BoxDecoration(color: KarryamColors.teal20, borderRadius: BorderRadius.circular(24)),
                child: const Icon(Icons.route_rounded, color: KarryamColors.midnightNavy, size: 36),
              ),
              const SizedBox(height: 28),
              Text('Set up once.\nMove with confidence.', style: Theme.of(context).textTheme.displaySmall?.copyWith(color: KarryamColors.pureWhite, fontWeight: FontWeight.w900, height: 1.05)),
              const SizedBox(height: 16),
              Text('We’ll guide you through your provider profile, operation mode, identity, vehicle and safety information.', style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.pureWhite.withValues(alpha: .78), height: 1.5)),
              const Spacer(),
              KarryamButton(label: 'Start provider setup', variant: KarryamButtonVariant.accent, onPressed: () {}),
              const SizedBox(height: 12),
              TextButton(onPressed: () {}, child: const Text('I’ll finish later', style: TextStyle(color: KarryamColors.pureWhite))),
            ],
          ),
        ),
      ),
    );
  }
}

class _OptionData {
  const _OptionData(this.icon, this.title, this.subtitle);
  final IconData icon;
  final String title;
  final String subtitle;
}

class _ChoiceScreen extends StatelessWidget {
  const _ChoiceScreen({required this.step, required this.title, required this.subtitle, required this.options});

  final int step;
  final String title;
  final String subtitle;
  final List<_OptionData> options;

  @override
  Widget build(BuildContext context) {
    return _SetupScaffold(
      step: step,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 16, 24, 28),
        children: [
          Text(title, style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: 8),
          Text(subtitle, style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 28),
          for (var i = 0; i < options.length; i++) ...[
            _SelectionCard(data: options[i], selected: i == 0),
            if (i < options.length - 1) const SizedBox(height: 12),
          ],
          const SizedBox(height: 32),
          KarryamButton(label: 'Continue', onPressed: () {}),
        ],
      ),
    );
  }
}

class _SelectionCard extends StatelessWidget {
  const _SelectionCard({required this.data, required this.selected});

  final _OptionData data;
  final bool selected;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(18),
      decoration: BoxDecoration(
        color: selected ? KarryamColors.teal10 : KarryamColors.pureWhite,
        borderRadius: BorderRadius.circular(KarryamRadius.lg),
        border: Border.all(color: selected ? KarryamColors.auroraTeal : KarryamColors.border, width: selected ? 1.5 : 1),
        boxShadow: selected ? KarryamElevation.subtle : null,
      ),
      child: Row(
        children: [
          Container(
            width: 52,
            height: 52,
            decoration: BoxDecoration(color: selected ? KarryamColors.teal20 : KarryamColors.navy10, borderRadius: BorderRadius.circular(16)),
            child: Icon(data.icon, color: KarryamColors.midnightNavy),
          ),
          const SizedBox(width: 14),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(data.title, style: Theme.of(context).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w800)),
                const SizedBox(height: 4),
                Text(data.subtitle, style: Theme.of(context).textTheme.bodyMedium?.copyWith(color: KarryamColors.indigoGray)),
              ],
            ),
          ),
          Icon(selected ? Icons.check_circle_rounded : Icons.radio_button_unchecked_rounded, color: selected ? KarryamColors.auroraTeal : KarryamColors.forgeGray),
        ],
      ),
    );
  }
}

class _ProfileDetailsScreen extends StatelessWidget {
  const _ProfileDetailsScreen();

  @override
  Widget build(BuildContext context) {
    return _SetupScaffold(
      step: 4,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 16, 24, 28),
        children: [
          Text('Tell us about you', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: 8),
          Text('Use your legal information so verification can match your provider profile.', style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 24),
          const KarryamTextField(label: 'Full legal name', hint: 'Daniel Ajushi', leading: Icon(Icons.badge_outlined)),
          const SizedBox(height: 16),
          const KarryamTextField(label: 'City', hint: 'Lagos', leading: Icon(Icons.location_city_outlined)),
          const SizedBox(height: 16),
          const KarryamTextField(label: 'Government ID number', hint: 'Enter ID number', leading: Icon(Icons.credit_card_rounded)),
          const SizedBox(height: 28),
          KarryamButton(label: 'Save and continue', onPressed: () {}),
        ],
      ),
    );
  }
}

class _PhotoGuidanceScreen extends StatelessWidget {
  const _PhotoGuidanceScreen();

  @override
  Widget build(BuildContext context) {
    return _SetupScaffold(
      step: 4,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 16, 24, 28),
        children: [
          Text('Take a clear provider photo', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: 8),
          Text('This photo is used for identity verification and your provider profile.', style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 28),
          Container(
            height: 250,
            decoration: BoxDecoration(color: KarryamColors.navy10, borderRadius: BorderRadius.circular(KarryamRadius.xl)),
            child: Stack(
              alignment: Alignment.center,
              children: [
                Container(width: 150, height: 190, decoration: BoxDecoration(border: Border.all(color: KarryamColors.auroraTeal, width: 2), borderRadius: BorderRadius.circular(76))),
                const Icon(Icons.person_outline_rounded, size: 96, color: KarryamColors.navy40),
              ],
            ),
          ),
          const SizedBox(height: 20),
          const KarryamNotice(title: 'For the best match', message: 'Use even lighting, remove sunglasses or helmets, and keep your full face inside the guide.'),
          const SizedBox(height: 24),
          KarryamButton(label: 'Open camera', leading: const Icon(Icons.camera_alt_outlined, size: 20), onPressed: () {}),
        ],
      ),
    );
  }
}

class _PhotoPreviewScreen extends StatelessWidget {
  const _PhotoPreviewScreen();

  @override
  Widget build(BuildContext context) {
    return _SetupScaffold(
      step: 4,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 16, 24, 28),
        children: [
          Text('Check your photo', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: 8),
          Text('Make sure your face is sharp, centered and clearly visible.', style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 28),
          Center(
            child: Container(
              width: 210,
              height: 210,
              decoration: BoxDecoration(color: KarryamColors.navy10, shape: BoxShape.circle, border: Border.all(color: KarryamColors.auroraTeal, width: 3)),
              child: const Icon(Icons.person_rounded, size: 112, color: KarryamColors.navy40),
            ),
          ),
          const SizedBox(height: 28),
          const KarryamNotice(title: 'Looks good', message: 'The image is clear enough to continue.', tone: KarryamNoticeTone.success),
          const SizedBox(height: 24),
          KarryamButton(label: 'Use this photo', onPressed: () {}),
          const SizedBox(height: 12),
          KarryamButton(label: 'Retake photo', variant: KarryamButtonVariant.secondary, onPressed: () {}),
        ],
      ),
    );
  }
}

class _BikeTypeScreen extends StatelessWidget {
  const _BikeTypeScreen();

  @override
  Widget build(BuildContext context) {
    const options = [
      _OptionData(Icons.two_wheeler_rounded, 'Dispatch bike', 'Motorcycle optimized for parcel delivery.'),
      _OptionData(Icons.electric_bike_rounded, 'Electric bike', 'Electric two-wheel delivery vehicle.'),
      _OptionData(Icons.pedal_bike_rounded, 'Bicycle', 'Non-motorized bicycle delivery.'),
      _OptionData(Icons.airport_shuttle_outlined, 'Tricycle', 'Three-wheel urban delivery vehicle.'),
    ];
    return _SetupScaffold(
      step: 5,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 16, 24, 28),
        children: [
          Text('Choose your delivery vehicle', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: 8),
          Text('You’ll add the registration and verification details next.', style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 24),
          for (var i = 0; i < options.length; i++) ...[
            _SelectionCard(data: options[i], selected: i == 0),
            const SizedBox(height: 12),
          ],
          const SizedBox(height: 16),
          KarryamButton(label: 'Continue', onPressed: () {}),
        ],
      ),
    );
  }
}

class _DriverInfoScreen extends StatelessWidget {
  const _DriverInfoScreen();

  @override
  Widget build(BuildContext context) {
    return _SetupScaffold(
      step: 6,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 16, 24, 28),
        children: [
          Text('Finish your provider details', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: 8),
          Text('Complete the information we need to initialize verification and vehicle review.', style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 24),
          const KarryamTextField(label: 'Driver licence number', hint: 'Enter licence number', leading: Icon(Icons.badge_outlined)),
          const SizedBox(height: 16),
          const KarryamTextField(label: 'Guarantor name', hint: 'Full legal name', leading: Icon(Icons.handshake_outlined)),
          const SizedBox(height: 16),
          const KarryamTextField(label: 'Emergency contact', hint: '+234 801 234 5678', keyboardType: TextInputType.phone, leading: Icon(Icons.emergency_outlined)),
          const SizedBox(height: 16),
          const KarryamNotice(title: 'Documents follow next', message: 'Licence, government ID, selfie and vehicle files are reviewed before you can go online.', tone: KarryamNoticeTone.info),
          const SizedBox(height: 28),
          KarryamButton(label: 'Review setup', onPressed: () {}),
        ],
      ),
    );
  }
}

class _ReviewScreen extends StatelessWidget {
  const _ReviewScreen();

  @override
  Widget build(BuildContext context) {
    const rows = [
      ('Account', 'Individual provider', Icons.person_outline_rounded),
      ('Service', 'Dispatch delivery', Icons.local_shipping_outlined),
      ('Profile', 'Daniel Ajushi', Icons.badge_outlined),
      ('Vehicle', 'Dispatch bike', Icons.two_wheeler_rounded),
      ('Safety', 'Guarantor + emergency contact', Icons.shield_outlined),
    ];
    return _SetupScaffold(
      step: 6,
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 16, 24, 28),
        children: [
          Text('Review your setup', style: Theme.of(context).textTheme.headlineLarge),
          const SizedBox(height: 8),
          Text('Check the details below before Karryam starts verification.', style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 24),
          Container(
            decoration: BoxDecoration(color: KarryamColors.pureWhite, borderRadius: BorderRadius.circular(KarryamRadius.lg), border: Border.all(color: KarryamColors.border)),
            child: Column(
              children: [
                for (var i = 0; i < rows.length; i++)
                  ListTile(
                    leading: Icon(rows[i].$3, color: KarryamColors.midnightNavy),
                    title: Text(rows[i].$1),
                    subtitle: Text(rows[i].$2),
                    trailing: const Icon(Icons.edit_outlined, size: 20),
                  ),
              ],
            ),
          ),
          const SizedBox(height: 28),
          KarryamButton(label: 'Submit for verification', onPressed: () {}),
        ],
      ),
    );
  }
}

class _SubmittingScreen extends StatelessWidget {
  const _SubmittingScreen();

  @override
  Widget build(BuildContext context) {
    return _SetupScaffold(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            const SizedBox.square(dimension: 54, child: CircularProgressIndicator(strokeWidth: 4, color: KarryamColors.auroraTeal)),
            const SizedBox(height: 28),
            Text('Creating your provider profile', style: Theme.of(context).textTheme.headlineLarge, textAlign: TextAlign.center),
            const SizedBox(height: 12),
            Text('We’re securely submitting your profile, safety information and initial verification setup. Don’t close the app.', style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray), textAlign: TextAlign.center),
            const SizedBox(height: 28),
            const KarryamNotice(title: 'Keep this screen open', message: 'Some verification steps are created in sequence. Karryam will confirm when the setup is ready to review.'),
          ],
        ),
      ),
    );
  }
}

class _PendingScreen extends StatelessWidget {
  const _PendingScreen();

  @override
  Widget build(BuildContext context) {
    return _SetupScaffold(
      child: ListView(
        padding: const EdgeInsets.fromLTRB(24, 52, 24, 28),
        children: [
          Center(
            child: Container(
              width: 92,
              height: 92,
              decoration: const BoxDecoration(color: KarryamColors.warningSurface, shape: BoxShape.circle),
              child: const Icon(Icons.hourglass_top_rounded, color: KarryamColors.warning, size: 40),
            ),
          ),
          const SizedBox(height: 28),
          Text('Your setup is under review', style: Theme.of(context).textTheme.headlineLarge, textAlign: TextAlign.center),
          const SizedBox(height: 12),
          Text('You can enter the app, but going online stays locked until required verification and vehicle checks are approved.', style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray), textAlign: TextAlign.center),
          const SizedBox(height: 28),
          const KarryamNotice(title: 'What happens next', message: 'Track identity, licence, vehicle, guarantor, emergency contact and face-verification status from Readiness.', tone: KarryamNoticeTone.warning),
          const SizedBox(height: 28),
          KarryamButton(label: 'Go to readiness', onPressed: () {}),
          const SizedBox(height: 12),
          KarryamButton(label: 'Enter provider app', variant: KarryamButtonVariant.secondary, onPressed: () {}),
        ],
      ),
    );
  }
}
