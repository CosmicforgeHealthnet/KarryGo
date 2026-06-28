import 'package:flutter/foundation.dart';

import '../../auth/models/provider_auth_models.dart';
import '../../home/data/provider_api.dart';
import '../models/customer_review.dart';

/// Loads the provider's bookings and buckets them into the My Trips tabs
/// (Completed / Ongoing / Cancelled). Backed by `GET /provider/bookings`.
class ProviderTripsController extends ChangeNotifier {
  ProviderTripsController({
    required ProviderApi api,
    required String? Function() accessToken,
  })  : _api = api,
        _accessToken = accessToken;

  final ProviderApi _api;
  final String? Function() _accessToken;

  bool _loading = false;
  String? _error;
  List<ProviderBooking> _bookings = const [];

  bool get isLoading => _loading;
  String? get error => _error;

  /// Completed (delivered + completed) trips, newest first.
  List<ProviderBooking> get completed => _filter(_completedStatuses);

  /// Ongoing trips — accepted through en-route-delivery, plus awaiting accept.
  List<ProviderBooking> get ongoing => _filter(_ongoingStatuses);

  /// Cancelled / unmatched trips.
  List<ProviderBooking> get cancelled => _filter(_cancelledStatuses);

  static const _completedStatuses = {'delivered', 'completed'};
  static const _ongoingStatuses = {
    'awaiting_acceptance',
    'accepted',
    'en_route_pickup',
    'arrived_at_pickup',
    'picked_up',
    'en_route_delivery',
  };
  static const _cancelledStatuses = {'cancelled', 'unmatched'};

  List<ProviderBooking> _filter(Set<String> statuses) {
    final list = _bookings.where((b) => statuses.contains(b.status)).toList()
      ..sort((a, b) => b.createdAt.compareTo(a.createdAt));
    return list;
  }

  Future<void> load() async {
    final token = _accessToken();
    if (token == null) return;
    _loading = true;
    _error = null;
    notifyListeners();
    try {
      _bookings = await _api.listBookings(accessToken: token);
    } catch (e) {
      _error = e.toString();
    } finally {
      _loading = false;
      notifyListeners();
    }
  }

  /// Fetches the customer review for a completed trip (null when none yet).
  Future<CustomerReview?> fetchReview(String bookingId) async {
    final token = _accessToken();
    if (token == null) return null;
    try {
      return await _api.getBookingReview(accessToken: token, bookingId: bookingId);
    } catch (_) {
      return null;
    }
  }
}
