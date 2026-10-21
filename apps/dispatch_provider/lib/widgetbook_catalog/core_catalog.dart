import 'package:flutter/material.dart';
import 'package:widgetbook/widgetbook.dart';

import '../design_system/karryam_design_system.dart';

WidgetbookFolder buildFoundationsCatalog() => WidgetbookFolder(
      name: '00 Foundations',
      children: [
        WidgetbookComponent(name: 'Brand palette', useCases: [WidgetbookUseCase(name: 'Official colors', builder: (_) => const _PalettePreview())]),
        WidgetbookComponent(name: 'Typography', useCases: [WidgetbookUseCase(name: 'Type scale', builder: (_) => const _TypographyPreview())]),
        WidgetbookComponent(name: 'Spacing & radius', useCases: [WidgetbookUseCase(name: 'Token scale', builder: (_) => const _SpacingPreview())]),
      ],
    );

WidgetbookFolder buildComponentsCatalog() => WidgetbookFolder(
      name: '01 Components',
      children: [
        WidgetbookComponent(
          name: 'Button',
          useCases: [
            WidgetbookUseCase(name: 'Primary', builder: (_) => _canvas(KarryamButton(label: 'Continue', onPressed: () {}))),
            WidgetbookUseCase(name: 'Accent', builder: (_) => _canvas(KarryamButton(label: 'Go online', variant: KarryamButtonVariant.accent, onPressed: () {}))),
            WidgetbookUseCase(name: 'Secondary', builder: (_) => _canvas(KarryamButton(label: 'Not now', variant: KarryamButtonVariant.secondary, onPressed: () {}))),
            WidgetbookUseCase(name: 'Loading', builder: (_) => _canvas(const KarryamButton(label: 'Processing', isLoading: true, onPressed: null))),
          ],
        ),
        WidgetbookComponent(
          name: 'Text field',
          useCases: [
            WidgetbookUseCase(name: 'Default', builder: (_) => _canvas(const KarryamTextField(label: 'Phone number', hint: '+234 801 234 5678', leading: Icon(Icons.phone_outlined)))),
            WidgetbookUseCase(name: 'Validation error', builder: (_) => _canvas(const KarryamTextField(label: 'Email address', hint: 'you@example.com', errorText: 'Enter a valid email address.', leading: Icon(Icons.mail_outline_rounded)))),
          ],
        ),
        WidgetbookComponent(name: 'System notice', useCases: [WidgetbookUseCase(name: 'Operational tones', builder: (_) => const _NoticePreview())]),
        WidgetbookComponent(name: 'Status chip', useCases: [WidgetbookUseCase(name: 'Operational states', builder: (_) => const _StatusPreview())]),
        WidgetbookComponent(
          name: 'Verification step card',
          useCases: [
            WidgetbookUseCase(name: 'Not started', builder: (_) => _canvas(KarryamVerificationStepCard(title: 'Identity', subtitle: 'Government ID and legal details', icon: Icons.badge_outlined, state: KarryamVerificationStepState.notStarted, onTap: () {}))),
            WidgetbookUseCase(name: 'In review', builder: (_) => _canvas(KarryamVerificationStepCard(title: 'Driver licence', subtitle: 'Licence and expiry check', icon: Icons.contact_page_outlined, state: KarryamVerificationStepState.submitted, onTap: () {}))),
            WidgetbookUseCase(name: 'Verified', builder: (_) => _canvas(KarryamVerificationStepCard(title: 'Emergency contact', subtitle: 'Safety contact confirmed', icon: Icons.health_and_safety_outlined, state: KarryamVerificationStepState.approved, onTap: () {}))),
            WidgetbookUseCase(name: 'Needs attention', builder: (_) => _canvas(KarryamVerificationStepCard(title: 'Vehicle', subtitle: 'Document needs replacement', icon: Icons.two_wheeler_rounded, state: KarryamVerificationStepState.rejected, onTap: () {}))),
          ],
        ),
      ],
    );

WidgetbookFolder buildProductionQueueCatalog() => WidgetbookFolder(
      name: '99 Production Queue',
      children: [
        WidgetbookComponent(name: 'Screen program', useCases: [WidgetbookUseCase(name: 'Master queue', builder: (_) => const _ProductionQueuePreview())]),
      ],
    );

Widget _canvas(Widget child) => Center(child: ConstrainedBox(constraints: const BoxConstraints(maxWidth: 390), child: Padding(padding: const EdgeInsets.all(24), child: child)));

class _PalettePreview extends StatelessWidget {
  const _PalettePreview();
  @override
  Widget build(BuildContext context) {
    const colors = [
      ('Midnight Navy', '#0B1F44', KarryamColors.midnightNavy),
      ('Aurora Teal', '#00C2A8', KarryamColors.auroraTeal),
      ('Pure White', '#FFFFFF', KarryamColors.pureWhite),
      ('Forge Gray', '#A7B0BB', KarryamColors.forgeGray),
      ('Indigo Gray', '#2D3A5B', KarryamColors.indigoGray),
    ];
    return ListView.separated(
      padding: const EdgeInsets.all(24),
      itemCount: colors.length,
      separatorBuilder: (_, __) => const SizedBox(height: 12),
      itemBuilder: (context, index) {
        final item = colors[index];
        return Container(
          padding: const EdgeInsets.all(16),
          decoration: BoxDecoration(color: Theme.of(context).colorScheme.surface, borderRadius: BorderRadius.circular(KarryamRadius.lg), border: Border.all(color: KarryamColors.border)),
          child: Row(children: [
            Container(width: 64, height: 64, decoration: BoxDecoration(color: item.$3, borderRadius: BorderRadius.circular(16), border: Border.all(color: KarryamColors.border))),
            const SizedBox(width: 16),
            Column(crossAxisAlignment: CrossAxisAlignment.start, children: [Text(item.$1, style: Theme.of(context).textTheme.titleMedium), const SizedBox(height: 4), Text(item.$2, style: Theme.of(context).textTheme.bodyMedium)]),
          ]),
        );
      },
    );
  }
}

class _TypographyPreview extends StatelessWidget {
  const _TypographyPreview();
  @override
  Widget build(BuildContext context) {
    final t = Theme.of(context).textTheme;
    return ListView(padding: const EdgeInsets.all(24), children: [
      Text('Movement intelligence', style: t.displayLarge),
      const SizedBox(height: 24),
      Text('Provider operations', style: t.headlineLarge),
      const SizedBox(height: 20),
      Text('Active mission', style: t.headlineMedium),
      const SizedBox(height: 16),
      Text('Pickup at Allen Avenue', style: t.titleLarge),
      const SizedBox(height: 12),
      Text('Clear actions, low friction and fast progress.', style: t.bodyLarge),
      const SizedBox(height: 8),
      Text('Secondary operational metadata', style: t.bodyMedium),
      const SizedBox(height: 12),
      Text('ACTION LABEL', style: t.labelMedium),
    ]);
  }
}

class _SpacingPreview extends StatelessWidget {
  const _SpacingPreview();
  @override
  Widget build(BuildContext context) {
    const items = [('XXS', 4.0), ('XS', 8.0), ('SM', 12.0), ('MD', 16.0), ('LG', 24.0), ('XL', 32.0), ('XXL', 48.0), ('XXXL', 64.0)];
    return ListView(padding: const EdgeInsets.all(24), children: [
      for (final item in items)
        Padding(padding: const EdgeInsets.only(bottom: 12), child: Row(children: [
          SizedBox(width: 56, child: Text(item.$1)),
          Container(width: item.$2 * 3, height: 12, decoration: BoxDecoration(color: KarryamColors.auroraTeal, borderRadius: BorderRadius.circular(999))),
          const SizedBox(width: 12),
          Text('${item.$2.toInt()} px'),
        ])),
    ]);
  }
}

class _NoticePreview extends StatelessWidget {
  const _NoticePreview();
  @override
  Widget build(BuildContext context) => ListView(padding: const EdgeInsets.all(24), children: const [
    KarryamNotice(title: 'Information', message: 'Operational context that helps the provider make the next decision.'),
    SizedBox(height: 12),
    KarryamNotice(title: 'Ready', message: 'The requirement has been confirmed by Karryam.', tone: KarryamNoticeTone.success),
    SizedBox(height: 12),
    KarryamNotice(title: 'Attention required', message: 'Review this item before continuing.', tone: KarryamNoticeTone.warning),
    SizedBox(height: 12),
    KarryamNotice(title: 'Action blocked', message: 'This operation cannot continue until the issue is resolved.', tone: KarryamNoticeTone.danger),
  ]);
}

class _StatusPreview extends StatelessWidget {
  const _StatusPreview();
  @override
  Widget build(BuildContext context) => const Center(child: Padding(padding: EdgeInsets.all(24), child: Wrap(spacing: 8, runSpacing: 8, children: [
    KarryamStatusChip(label: 'Offline'),
    KarryamStatusChip(label: 'Online', tone: KarryamStatusTone.active),
    KarryamStatusChip(label: 'Verified', tone: KarryamStatusTone.success),
    KarryamStatusChip(label: 'Pending', tone: KarryamStatusTone.warning),
    KarryamStatusChip(label: 'Blocked', tone: KarryamStatusTone.danger),
    KarryamStatusChip(label: 'Syncing', tone: KarryamStatusTone.info),
  ])));
}

class _ProductionQueuePreview extends StatelessWidget {
  const _ProductionQueuePreview();
  @override
  Widget build(BuildContext context) {
    const groups = [
      ('Launch & Auth', 'BUILT'),
      ('Onboarding & Provider Setup', 'BUILT'),
      ('Verification', 'BUILT'),
      ('Vehicle', 'NEXT'),
      ('Home & Availability', 'QUEUED'),
      ('Opportunities', 'QUEUED'),
      ('Activity & Trips', 'QUEUED'),
      ('Mission Pickup', 'QUEUED'),
      ('Mission Delivery', 'QUEUED'),
      ('Multi-stop & Parcels', 'QUEUED'),
      ('Secure Handover & Evidence', 'QUEUED'),
      ('Recovery, Returns & Transfer', 'QUEUED'),
      ('Route Integrity & Offline', 'QUEUED'),
      ('Wallet, Earnings & Transactions', 'QUEUED'),
      ('Bank Accounts, Withdrawals & PIN', 'QUEUED'),
      ('Notifications', 'QUEUED'),
      ('Account, Security & Privacy', 'QUEUED'),
      ('Support, Chat & SOS', 'QUEUED'),
      ('Global States & Overlays', 'QUEUED'),
    ];
    return ListView.separated(
      padding: const EdgeInsets.all(24),
      itemCount: groups.length,
      separatorBuilder: (_, __) => const Divider(height: 1),
      itemBuilder: (context, index) => ListTile(
        contentPadding: const EdgeInsets.symmetric(vertical: 4),
        leading: CircleAvatar(backgroundColor: index < 3 ? KarryamColors.teal20 : KarryamColors.navy10, foregroundColor: KarryamColors.midnightNavy, child: Text('${index + 1}')),
        title: Text(groups[index].$1),
        subtitle: Text(groups[index].$2),
      ),
    );
  }
}
