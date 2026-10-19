import '../../../core/data/driver_client.dart';
import '../../home/models/provider_booking.dart';

/// The rider's review of a trip, as far as the driver may see it.
class TripReview {
  const TripReview({this.rating, this.text = ''});
  final int? rating;
  final String text;

  static TripReview? fromView(Map<String, dynamic> view) {
    final theirs = asMap(view['theirs']);
    if (theirs['submitted'] != true) return null;
    return TripReview(
      rating: (theirs['rating'] as num?)?.toInt(),
      text: (theirs['review_text'] as String?) ?? '',
    );
  }
}

/// The driver's finished rides (taxi `GET /provider/trips`).
class TripsApi {
  TripsApi(this._client);
  final DriverClient _client;

  Future<List<ProviderBooking>> listHistory() async {
    final data = await _client.get('/provider/trips');
    return asList(data, 'bookings').map(ProviderBooking.fromJson).toList();
  }

  Future<TripReview?> passengerReview(String bookingId) async {
    final view = await _client.getMap('/provider/bookings/$bookingId/review');
    return TripReview.fromView(view);
  }
}
