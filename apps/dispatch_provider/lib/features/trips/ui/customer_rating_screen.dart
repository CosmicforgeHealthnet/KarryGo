import 'package:flutter/material.dart';

import '../../../core/theme/theme_extensions.dart';
import '../data/trip_model.dart';

class CustomerRatingScreen extends StatelessWidget {
  const CustomerRatingScreen({super.key, required this.trip});
  final TripModel trip;

  @override
  Widget build(BuildContext context) {
    final colors = context.palette;
    final rating = trip.customerRating ?? 0;

    return Scaffold(
      backgroundColor: colors.background,
      body: SafeArea(
        child: Column(
          children: [
            _buildTopBar(context),
            Expanded(
              child: SingleChildScrollView(
                padding: const EdgeInsets.fromLTRB(20, 8, 20, 24),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.stretch,
                  children: [
                    const SizedBox(height: 12),
                    Center(
                      child: Text(
                        rating.toStringAsFixed(1),
                        style: TextStyle(
                          fontSize: 56,
                          fontWeight: FontWeight.w800,
                          color: colors.text,
                          letterSpacing: -2.0,
                          height: 1.0,
                        ),
                      ),
                    ),
                    const SizedBox(height: 14),
                    Row(
                      mainAxisAlignment: MainAxisAlignment.center,
                      children: List.generate(
                        5,
                        (i) => Padding(
                          padding: const EdgeInsets.symmetric(horizontal: 3),
                          child: Icon(
                            i < rating.round()
                                ? Icons.star_rounded
                                : Icons.star_outline_rounded,
                            size: 32,
                            color: colors.warning,
                          ),
                        ),
                      ),
                    ),
                    if (trip.customerFeedback != null) ...[
                      const SizedBox(height: 20),
                      Text(
                        '"${trip.customerFeedback}"',
                        textAlign: TextAlign.center,
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.w500,
                          fontStyle: FontStyle.italic,
                          color: colors.text,
                          height: 1.5,
                        ),
                      ),
                    ],
                    const SizedBox(height: 24),
                    _buildCustomerCard(context),
                    const SizedBox(height: 20),
                    _buildFeedbackCard(context),
                    const SizedBox(height: 24),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildTopBar(BuildContext context) {
    final colors = context.palette;
    return Padding(
      padding: const EdgeInsets.fromLTRB(12, 8, 12, 4),
      child: Stack(
        alignment: Alignment.center,
        children: [
          Align(
            alignment: Alignment.centerLeft,
            child: GestureDetector(
              onTap: () => Navigator.of(context).pop(),
              behavior: HitTestBehavior.opaque,
              child: Padding(
                padding: const EdgeInsets.all(8),
                child: Icon(
                  Icons.arrow_back_rounded,
                  size: 22,
                  color: colors.text,
                ),
              ),
            ),
          ),
          Text(
            'Customer rating',
            style: TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w700,
              color: colors.text,
              letterSpacing: -0.2,
            ),
          ),
          const SizedBox(width: 40),
        ],
      ),
    );
  }

  Widget _buildCustomerCard(BuildContext context) {
    final colors = context.palette;
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider),
      ),
      child: Row(
        children: [
          Container(
            width: 48,
            height: 48,
            alignment: Alignment.center,
            decoration: BoxDecoration(
              color: colors.navy10,
              shape: BoxShape.circle,
            ),
            child: Icon(
              Icons.person_rounded,
              size: 24,
              color: colors.textMuted,
            ),
          ),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(
                  trip.receiverName.isNotEmpty
                      ? trip.receiverName
                      : trip.customerName,
                  style: TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w700,
                    color: colors.text,
                    letterSpacing: -0.1,
                  ),
                ),
                const SizedBox(height: 3),
                Row(
                  children: [
                    Icon(
                      Icons.verified_rounded,
                      size: 13,
                      color: colors.brandTeal500,
                    ),
                    const SizedBox(width: 4),
                    Text(
                      'Verified customer',
                      style: TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w600,
                        color: colors.brandTeal600,
                      ),
                    ),
                  ],
                ),
              ],
            ),
          ),
          if (trip.completedAt != null)
            Text(
              _formatShortDate(trip.completedAt!),
              style: TextStyle(
                fontSize: 12,
                color: colors.textMuted,
              ),
            ),
        ],
      ),
    );
  }

  Widget _buildFeedbackCard(BuildContext context) {
    final colors = context.palette;
    if (trip.customerFeedback == null) return const SizedBox.shrink();

    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.surface,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: colors.divider),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(
            'Customer feedback',
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w700,
              color: colors.textMuted,
              letterSpacing: 0.2,
            ),
          ),
          const SizedBox(height: 10),
          Text(
            '"${trip.customerFeedback}"',
            style: TextStyle(
              fontSize: 14,
              color: colors.text,
              height: 1.55,
              fontStyle: FontStyle.italic,
            ),
          ),
        ],
      ),
    );
  }

  static String _formatShortDate(DateTime dt) {
    const months = [
      'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
      'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
    ];
    return '${dt.day} ${months[dt.month - 1]} ${dt.year}';
  }
}