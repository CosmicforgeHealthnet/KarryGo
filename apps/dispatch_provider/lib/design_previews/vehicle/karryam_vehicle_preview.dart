import 'package:flutter/material.dart';

import '../../design_system/karryam_design_system.dart';

enum KarryamVehiclePreviewState {
  hub,
  chooseType,
  details,
  documents,
  registrationUpload,
  insuranceUpload,
  pending,
  rejected,
  verified,
  suspended,
}

class KarryamVehiclePreview extends StatelessWidget {
  const KarryamVehiclePreview({super.key, required this.state});

  final KarryamVehiclePreviewState state;

  @override
  Widget build(BuildContext context) => switch (state) {
        KarryamVehiclePreviewState.hub => const _VehicleHub(),
        KarryamVehiclePreviewState.chooseType => const _ChooseVehicleType(),
        KarryamVehiclePreviewState.details => const _VehicleDetails(),
        KarryamVehiclePreviewState.documents => const _VehicleDocuments(),
        KarryamVehiclePreviewState.registrationUpload => const _DocumentUpload(title: 'Vehicle registration', subtitle: 'Upload a clear JPEG, PNG or PDF copy of your registration document.'),
        KarryamVehiclePreviewState.insuranceUpload => const _DocumentUpload(title: 'Insurance document', subtitle: 'Upload your active insurance document and confirm its expiry date.'),
        KarryamVehiclePreviewState.pending => const _VehicleState(title: 'Vehicle under review', message: 'Your vehicle and documents were submitted successfully. You can continue preparing your account while Karryam reviews them.', icon: Icons.hourglass_top_rounded, tone: KarryamNoticeTone.info),
        KarryamVehiclePreviewState.rejected => const _VehicleRejected(),
        KarryamVehiclePreviewState.verified => const _VehicleVerified(),
        KarryamVehiclePreviewState.suspended => const _VehicleState(title: 'Vehicle temporarily unavailable', message: 'This vehicle cannot be used for new dispatch opportunities until the restriction is resolved.', icon: Icons.block_rounded, tone: KarryamNoticeTone.danger),
      };
}

class _Screen extends StatelessWidget {
  const _Screen({required this.title, required this.child, this.subtitle});
  final String title;
  final String? subtitle;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: KarryamColors.backgroundSubtle,
      appBar: AppBar(title: Text(title)),
      body: SafeArea(
        child: Center(
          child: ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 430),
            child: ListView(
              padding: const EdgeInsets.fromLTRB(24, 20, 24, 32),
              children: [
                if (subtitle != null) ...[
                  Text(subtitle!, style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
                  const SizedBox(height: KarryamSpacing.lg),
                ],
                child,
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _VehicleHub extends StatelessWidget {
  const _VehicleHub();

  @override
  Widget build(BuildContext context) {
    return _Screen(
      title: 'Vehicle',
      subtitle: 'Your verified vehicle controls whether you can go online for dispatch work.',
      child: Column(
        children: [
          Container(
            padding: const EdgeInsets.all(20),
            decoration: BoxDecoration(color: KarryamColors.midnightNavy, borderRadius: BorderRadius.circular(KarryamRadius.xl)),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                const Row(
                  mainAxisAlignment: MainAxisAlignment.spaceBetween,
                  children: [
                    Icon(Icons.two_wheeler_rounded, color: KarryamColors.pureWhite, size: 34),
                    KarryamStatusChip(label: 'Pending review', tone: KarryamStatusTone.warning),
                  ],
                ),
                const SizedBox(height: 28),
                Text('Honda CB125', style: Theme.of(context).textTheme.headlineSmall?.copyWith(color: KarryamColors.pureWhite, fontWeight: FontWeight.w900)),
                const SizedBox(height: 4),
                Text('Dispatch bike • ABC-123XY', style: Theme.of(context).textTheme.bodyMedium?.copyWith(color: KarryamColors.pureWhite.withValues(alpha: .72))),
              ],
            ),
          ),
          const SizedBox(height: 20),
          const _ActionTile(icon: Icons.description_outlined, title: 'Vehicle documents', subtitle: 'Registration and insurance'),
          const SizedBox(height: 12),
          const _ActionTile(icon: Icons.edit_outlined, title: 'Vehicle details', subtitle: 'Brand, model, year, colour and plate'),
          const SizedBox(height: 12),
          const _ActionTile(icon: Icons.verified_user_outlined, title: 'Verification history', subtitle: 'Track review and rejection reasons'),
        ],
      ),
    );
  }
}

class _ChooseVehicleType extends StatelessWidget {
  const _ChooseVehicleType();

  @override
  Widget build(BuildContext context) {
    const items = [
      ('Motorcycle', Icons.two_wheeler_rounded),
      ('Dispatch bike', Icons.delivery_dining_rounded),
      ('Scooter', Icons.moped_rounded),
      ('Tricycle', Icons.electric_rickshaw_rounded),
      ('Bicycle', Icons.pedal_bike_rounded),
      ('Electric bike', Icons.electric_bike_rounded),
    ];
    return _Screen(
      title: 'Choose vehicle type',
      subtitle: 'Select the vehicle you will use for Karryam dispatch jobs.',
      child: Column(
        children: [
          for (final item in items) ...[
            _SelectCard(title: item.$1, icon: item.$2),
            const SizedBox(height: 12),
          ],
          const SizedBox(height: 8),
          KarryamButton(label: 'Continue', onPressed: () {}),
        ],
      ),
    );
  }
}

class _VehicleDetails extends StatelessWidget {
  const _VehicleDetails();

  @override
  Widget build(BuildContext context) {
    return _Screen(
      title: 'Vehicle details',
      subtitle: 'Enter the details exactly as they appear on your vehicle documents.',
      child: Column(
        children: [
          const KarryamTextField(label: 'Brand', hint: 'Honda'),
          const SizedBox(height: 16),
          const KarryamTextField(label: 'Model', hint: 'CB125'),
          const SizedBox(height: 16),
          const KarryamTextField(label: 'Year', hint: '2024', keyboardType: TextInputType.number),
          const SizedBox(height: 16),
          const KarryamTextField(label: 'Colour', hint: 'Black'),
          const SizedBox(height: 16),
          const KarryamTextField(label: 'Plate number', hint: 'ABC-123XY'),
          const SizedBox(height: 24),
          KarryamButton(label: 'Review vehicle', onPressed: () {}),
        ],
      ),
    );
  }
}

class _VehicleDocuments extends StatelessWidget {
  const _VehicleDocuments();

  @override
  Widget build(BuildContext context) {
    return _Screen(
      title: 'Vehicle documents',
      subtitle: 'Both documents must be valid before the vehicle can become verified.',
      child: const Column(
        children: [
          _DocumentCard(title: 'Registration', status: 'Uploaded', tone: KarryamStatusTone.success),
          SizedBox(height: 12),
          _DocumentCard(title: 'Insurance', status: 'Required', tone: KarryamStatusTone.warning),
        ],
      ),
    );
  }
}

class _DocumentUpload extends StatelessWidget {
  const _DocumentUpload({required this.title, required this.subtitle});
  final String title;
  final String subtitle;

  @override
  Widget build(BuildContext context) {
    return _Screen(
      title: title,
      subtitle: subtitle,
      child: Column(
        children: [
          Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(vertical: 42, horizontal: 24),
            decoration: BoxDecoration(
              color: KarryamColors.pureWhite,
              borderRadius: BorderRadius.circular(KarryamRadius.xl),
              border: Border.all(color: KarryamColors.border),
            ),
            child: Column(
              children: [
                const Icon(Icons.cloud_upload_outlined, size: 42, color: KarryamColors.auroraTeal),
                const SizedBox(height: 12),
                Text('Tap to upload document', style: Theme.of(context).textTheme.titleMedium),
                const SizedBox(height: 6),
                Text('JPEG, PNG or PDF • maximum 5 MB', style: Theme.of(context).textTheme.bodySmall?.copyWith(color: KarryamColors.indigoGray)),
              ],
            ),
          ),
          const SizedBox(height: 24),
          KarryamButton(label: 'Continue', onPressed: () {}),
        ],
      ),
    );
  }
}

class _VehicleRejected extends StatelessWidget {
  const _VehicleRejected();

  @override
  Widget build(BuildContext context) {
    return _Screen(
      title: 'Vehicle needs attention',
      child: Column(
        children: [
          const KarryamNotice(
            title: 'Registration document rejected',
            message: 'The uploaded registration image is not readable. Upload a clearer copy showing the full document.',
            tone: KarryamNoticeTone.danger,
          ),
          const SizedBox(height: 20),
          const _DocumentCard(title: 'Registration', status: 'Rejected', tone: KarryamStatusTone.danger),
          const SizedBox(height: 12),
          const _DocumentCard(title: 'Insurance', status: 'Verified', tone: KarryamStatusTone.success),
          const SizedBox(height: 24),
          KarryamButton(label: 'Replace registration document', onPressed: () {}),
        ],
      ),
    );
  }
}

class _VehicleVerified extends StatelessWidget {
  const _VehicleVerified();

  @override
  Widget build(BuildContext context) {
    return _Screen(
      title: 'Vehicle verified',
      child: Column(
        children: [
          Container(
            width: 88,
            height: 88,
            decoration: const BoxDecoration(color: KarryamColors.teal10, shape: BoxShape.circle),
            alignment: Alignment.center,
            child: const Icon(Icons.verified_rounded, size: 44, color: KarryamColors.auroraTeal),
          ),
          const SizedBox(height: 20),
          Text('Honda CB125 is ready', style: Theme.of(context).textTheme.headlineSmall, textAlign: TextAlign.center),
          const SizedBox(height: 8),
          Text('This vehicle can now be used when your provider account is otherwise ready to go online.', textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 24),
          KarryamButton(label: 'Return to vehicle', onPressed: () {}),
        ],
      ),
    );
  }
}

class _VehicleState extends StatelessWidget {
  const _VehicleState({required this.title, required this.message, required this.icon, required this.tone});
  final String title;
  final String message;
  final IconData icon;
  final KarryamNoticeTone tone;

  @override
  Widget build(BuildContext context) {
    return _Screen(
      title: 'Vehicle status',
      child: Column(
        children: [
          Icon(icon, size: 58, color: tone == KarryamNoticeTone.danger ? KarryamColors.danger : KarryamColors.auroraTeal),
          const SizedBox(height: 20),
          Text(title, style: Theme.of(context).textTheme.headlineSmall, textAlign: TextAlign.center),
          const SizedBox(height: 8),
          Text(message, textAlign: TextAlign.center, style: Theme.of(context).textTheme.bodyLarge?.copyWith(color: KarryamColors.indigoGray)),
          const SizedBox(height: 24),
          KarryamButton(label: 'View vehicle', onPressed: () {}),
        ],
      ),
    );
  }
}

class _ActionTile extends StatelessWidget {
  const _ActionTile({required this.icon, required this.title, required this.subtitle});
  final IconData icon;
  final String title;
  final String subtitle;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(color: KarryamColors.pureWhite, borderRadius: BorderRadius.circular(KarryamRadius.lg), border: Border.all(color: KarryamColors.border)),
      child: Row(children: [
        Icon(icon, color: KarryamColors.midnightNavy),
        const SizedBox(width: 14),
        Expanded(child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [Text(title, style: Theme.of(context).textTheme.titleSmall), Text(subtitle, style: Theme.of(context).textTheme.bodySmall?.copyWith(color: KarryamColors.indigoGray))])),
        const Icon(Icons.chevron_right_rounded),
      ]),
    );
  }
}

class _SelectCard extends StatelessWidget {
  const _SelectCard({required this.title, required this.icon});
  final String title;
  final IconData icon;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(color: KarryamColors.pureWhite, borderRadius: BorderRadius.circular(KarryamRadius.lg), border: Border.all(color: KarryamColors.border)),
      child: Row(children: [Icon(icon, color: KarryamColors.midnightNavy, size: 28), const SizedBox(width: 14), Expanded(child: Text(title, style: Theme.of(context).textTheme.titleMedium)), const Icon(Icons.radio_button_unchecked_rounded, color: KarryamColors.forgeGray)]),
    );
  }
}

class _DocumentCard extends StatelessWidget {
  const _DocumentCard({required this.title, required this.status, required this.tone});
  final String title;
  final String status;
  final KarryamStatusTone tone;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(color: KarryamColors.pureWhite, borderRadius: BorderRadius.circular(KarryamRadius.lg), border: Border.all(color: KarryamColors.border)),
      child: Row(children: [
        const Icon(Icons.description_outlined, color: KarryamColors.midnightNavy),
        const SizedBox(width: 14),
        Expanded(child: Text(title, style: Theme.of(context).textTheme.titleSmall)),
        KarryamStatusChip(label: status, tone: tone),
      ]),
    );
  }
}
