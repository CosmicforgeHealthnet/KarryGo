import 'package:flutter/material.dart';
import '../../../../core/theme/theme_extensions.dart';
import '../../../../shared/widgets/journey_icon.dart';

class HomeHeaderBar extends StatelessWidget {
  const HomeHeaderBar({
    super.key,
    this.avatarImageProvider,
    this.onOpenNotifications,
    this.onOpenDelivery,
    this.onOpenAccount,
  });
  final ImageProvider? avatarImageProvider;
  final VoidCallback? onOpenNotifications;
  final VoidCallback? onOpenDelivery;
  final VoidCallback? onOpenAccount;
  @override
  Widget build(BuildContext context) {
    final c = context.palette;
    return Row(
      children: [
        Image.asset('assets/figma/splash_logo.png', width: 28, height: 28),
        const SizedBox(width: 10),
        Expanded(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('Karryam', style: Theme.of(context).textTheme.titleMedium),
              Text(
                'RIDER',
                style: Theme.of(context).textTheme.labelSmall?.copyWith(
                  letterSpacing: 2,
                  color: c.textMuted,
                  fontSize: 9,
                ),
              ),
            ],
          ),
        ),
        if (onOpenDelivery != null)
          IconButton(
            tooltip: 'Current delivery',
            onPressed: onOpenDelivery,
            icon: const JourneyIcon(JourneySymbol.parcel, size: 24),
          ),
        IconButton(
          tooltip: 'Notifications',
          onPressed: onOpenNotifications,
          icon: const Icon(Icons.notifications_none_rounded, size: 23),
        ),
        IconButton(
          tooltip: 'Your account',
          onPressed: onOpenAccount,
          icon: CircleAvatar(
            radius: 19,
            backgroundColor: c.surfaceElevated,
            backgroundImage: avatarImageProvider,
            child: avatarImageProvider == null
                ? JourneyIcon(JourneySymbol.profile, size: 22, color: c.text)
                : null,
          ),
        ),
      ],
    );
  }
}
