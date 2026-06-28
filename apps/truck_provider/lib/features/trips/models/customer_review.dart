import 'package:flutter/foundation.dart';

/// The customer's review of a completed trip, shown in the "Customer Review"
/// section of the trip detail screen.
@immutable
class CustomerReview {
  const CustomerReview({
    required this.rating,
    this.reviewText = '',
    this.recommendsDriver,
  });

  final int rating;
  final String reviewText;
  final bool? recommendsDriver;

  factory CustomerReview.fromJson(Map<String, dynamic> j) => CustomerReview(
        rating: (j['rating'] as num? ?? 0).toInt(),
        reviewText: j['review_text'] as String? ?? '',
        recommendsDriver: j['recommends_driver'] as bool?,
      );
}
