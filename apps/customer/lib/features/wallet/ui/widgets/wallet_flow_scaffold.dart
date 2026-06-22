import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';

/// Shared scaffold for wallet flow screens, mirroring the hauling flow scaffold.
/// Provides a consistent app-bar with a back button and the surface background.
class WalletFlowScaffold extends StatelessWidget {
  const WalletFlowScaffold({
    super.key,
    required this.title,
    required this.body,
    this.onBack,
    this.bottom,
    this.backgroundColor = CustomerFigmaColors.surface,
    this.actions,
  });

  final String title;
  final Widget body;
  final VoidCallback? onBack;
  final Widget? bottom;
  final Color backgroundColor;
  final List<Widget>? actions;

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: backgroundColor,
      appBar: AppBar(
        backgroundColor: backgroundColor,
        elevation: 0,
        scrolledUnderElevation: 0,
        leading: FigmaBackButton(
          onPressed: onBack ?? () => Navigator.of(context).maybePop(),
        ),
        title: Text(
          title,
          style: const TextStyle(
            color: CustomerFigmaColors.text,
            fontWeight: FontWeight.w800,
            fontSize: 18,
          ),
        ),
        centerTitle: true,
        actions: actions,
      ),
      body: SafeArea(top: false, child: body),
      bottomNavigationBar: switch (bottom) {
        final Widget b => SafeArea(
            minimum: const EdgeInsets.fromLTRB(20, 0, 20, 16),
            child: b,
          ),
        null => null,
      },
    );
  }
}

/// Section heading used across wallet screens.
class WalletSectionLabel extends StatelessWidget {
  const WalletSectionLabel(this.text, {super.key, this.trailing});

  final String text;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Text(
          text,
          style: const TextStyle(
            color: CustomerFigmaColors.text,
            fontSize: 16,
            fontWeight: FontWeight.w800,
          ),
        ),
        ?trailing,
      ],
    );
  }
}
