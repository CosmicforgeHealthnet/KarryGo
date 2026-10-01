import 'package:flutter/material.dart';

import '../../../../core/theme/theme_extensions.dart';

class HomeHeaderBar extends StatelessWidget {
  const HomeHeaderBar({
    super.key,
    this.avatarImageProvider,
    this.onOpenNotifications,
  });

  final ImageProvider? avatarImageProvider;
  final VoidCallback? onOpenNotifications;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return Row(
      children: [
        const _KarryamLogo(),
        const SizedBox(width: 8),
        // Text(
        //   'Karryam',
        //   style: TextStyle(
        //     fontSize: 20,
        //     fontWeight: FontWeight.w800,
        //     color: colors.text,
        //     letterSpacing: -0.3,
        //   ),
        // ),
        const Spacer(),
        GestureDetector(
          onTap: onOpenNotifications,
          child: CircleAvatar(
            radius: 19,
            backgroundColor: colors.navy20,
            backgroundImage: avatarImageProvider,
            child: avatarImageProvider == null
                ? Icon(Icons.person, color: colors.text, size: 20)
                : null,
          ),
        ),
      ],
    );
  }
}

class _KarryamLogo extends StatelessWidget {
  const _KarryamLogo();

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    return SizedBox(
      width: 30,
      height: 30,
      child: Image.asset(
        'assets/figma/splash_logo.png',
        fit: BoxFit.contain,
        errorBuilder: (context, error, stackTrace) => Container(
          alignment: Alignment.center,
          decoration: BoxDecoration(
            color: colors.brandNavy900,
            borderRadius: BorderRadius.circular(8),
          ),
          child: Text(
            'K',
            style: TextStyle(
              color: colors.brandTeal500,
              fontSize: 18,
              fontWeight: FontWeight.w900,
              height: 1,
            ),
          ),
        ),
      ),
    );
  }
}