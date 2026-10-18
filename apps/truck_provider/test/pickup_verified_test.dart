import 'package:flutter_test/flutter_test.dart';
import 'package:truck_provider/features/auth/models/provider_auth_models.dart';

Map<String, dynamic> _json([Map<String, dynamic> extra = const {}]) => {
  'id': 'b1',
  'pickup_address': 'Ikeja',
  'pickup_lat': 6.6,
  'pickup_lng': 3.35,
  'dropoff_address': 'Lekki',
  'dropoff_lat': 6.44,
  'dropoff_lng': 3.47,
  'cargo_type': 'general',
  'cargo_weight_kg': 100,
  'status': 'arrived_at_pickup',
  'created_at': '2026-10-18T10:00:00Z',
  'receiver_name': 'Ada Obi',
  ...extra,
};

void main() {
  test('pickup is unverified until the server reports a time', () {
    expect(ProviderBooking.fromJson(_json()).pickupVerified, isFalse);
    final verified = ProviderBooking.fromJson(
      _json({'pickup_verified_at': '2026-10-18T10:05:00Z'}),
    );
    expect(verified.pickupVerified, isTrue);
  });

  test('the receiver is never presented as the customer', () {
    final b = ProviderBooking.fromJson(_json());
    expect(b.displayName, 'Customer');
    expect(b.receiverLine, 'Receiver: Ada Obi');
  });
}
