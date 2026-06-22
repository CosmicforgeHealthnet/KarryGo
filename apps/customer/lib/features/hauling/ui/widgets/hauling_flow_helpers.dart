import 'package:flutter/material.dart';

import '../../../../shared/widgets/figma_customer_widgets.dart';

/// Wraps each hauling flow view in a consistent scaffold layout.
Widget haulingFlowScaffold({
  required Widget body,
  String? title,
  VoidCallback? onBack,
  Widget? bottom,
  Color bg = CustomerFigmaColors.surface,
}) {
  return Scaffold(
    backgroundColor: bg,
    appBar: title != null
        ? AppBar(
            backgroundColor: bg,
            elevation: 0,
            surfaceTintColor: Colors.transparent,
            leading: onBack != null
                ? IconButton(
                    onPressed: onBack,
                    icon: const Icon(Icons.arrow_back_rounded),
                    color: CustomerFigmaColors.text,
                  )
                : null,
            title: Text(
              title,
              style: const TextStyle(
                color: CustomerFigmaColors.text,
                fontWeight: FontWeight.w800,
                fontSize: 17,
              ),
            ),
          )
        : null,
    body: SafeArea(
      child: Padding(
        padding: const EdgeInsets.fromLTRB(20, 8, 20, 20),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Expanded(child: body),
            ?bottom,
          ],
        ),
      ),
    ),
  );
}

Widget haulingSectionLabel(String text) => Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Text(
        text,
        style: const TextStyle(
          color: CustomerFigmaColors.text,
          fontSize: 13,
          fontWeight: FontWeight.w800,
        ),
      ),
    );

Widget haulingFareRow(String label, int kobo) => Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(
            label,
            style: const TextStyle(color: CustomerFigmaColors.muted, fontSize: 13),
          ),
          Text(
            '₦${(kobo / 100).toStringAsFixed(0)}',
            style: const TextStyle(
              color: CustomerFigmaColors.text,
              fontSize: 13,
              fontWeight: FontWeight.w700,
            ),
          ),
        ],
      ),
    );
