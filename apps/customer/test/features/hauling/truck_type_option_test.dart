import 'package:customer/features/hauling/models/hauling_models.dart';
import 'package:flutter_test/flutter_test.dart';

/// The truck types the hauling backend accepts, mirroring `ValidTruckTypes` in
/// `services/driver-hauling-service/internal/features/provider_profile/models/models.go`
/// (and `ValidPreferredTruckTypes` in the booking models, which is validated on
/// booking creation).
///
/// This list is duplicated deliberately: it is the contract these tests pin. If
/// the backend adds a truck type, update this set and the enum together.
const _backendTruckTypes = <String>{
  'flatbed',
  'container',
  'tipper',
  'van',
  'refrigerated',
  'pickup',
  'box',
  'tanker',
  'trailer',
  'dump',
  'lowbed',
  'crane',
  'other',
};

void main() {
  group('HaulingTruckTypeOption', () {
    // The matcher compares a booking's preferred_truck_type to the provider's
    // trucks.truck_type with exact string equality, so anything the customer app
    // can emit that is not a real backend slug silently matches no one.
    test('every apiValue is a truck type the backend knows', () {
      for (final option in HaulingTruckTypeOption.values) {
        if (option == HaulingTruckTypeOption.other) continue;
        expect(
          _backendTruckTypes,
          contains(option.apiValue),
          reason:
              '${option.name} sends "${option.apiValue}", which the backend '
              'does not accept — bookings using it would never match.',
        );
      }
    });

    // A many-to-one mapping is what originally hid "Van" from customers and made
    // pickup/box/tanker/trailer/dump/lowbed/crane providers unmatchable.
    test('apiValues are unique, so no two labels collapse onto one slug', () {
      final seen = <String, String>{};
      for (final option in HaulingTruckTypeOption.values) {
        if (option == HaulingTruckTypeOption.other) continue;
        final previous = seen[option.apiValue];
        expect(
          previous,
          isNull,
          reason:
              '${option.name} and $previous both send "${option.apiValue}"; '
              'one of them can never be expressed to the backend.',
        );
        seen[option.apiValue] = option.name;
      }
    });

    // Providers can register any of these, so a customer must be able to ask for
    // any of them — otherwise those providers are unreachable.
    test('covers every bookable backend truck type', () {
      final covered = HaulingTruckTypeOption.values
          .where((o) => o != HaulingTruckTypeOption.other)
          .map((o) => o.apiValue)
          .toSet();
      // 'other' is excluded: no provider registers a truck as `other`, so it is
      // not bookable. The enum's `other` maps to '' (any truck) instead.
      expect(covered, containsAll(_backendTruckTypes.difference({'other'})));
    });

    test('"Any / Not sure" sends an empty value so the type filter is skipped',
        () {
      expect(HaulingTruckTypeOption.other.apiValue, isEmpty);
    });

    test('fromApiValue round-trips every option exactly', () {
      for (final option in HaulingTruckTypeOption.values) {
        if (option == HaulingTruckTypeOption.other) continue;
        expect(
          HaulingTruckTypeOption.fromApiValue(option.apiValue),
          option,
          reason: '${option.apiValue} should map back to ${option.name}',
        );
      }
    });

    test('fromApiValue returns null for empty and unknown values', () {
      expect(HaulingTruckTypeOption.fromApiValue(''), isNull);
      expect(HaulingTruckTypeOption.fromApiValue('spaceship'), isNull);
    });

    test('every option has a non-empty display label', () {
      for (final option in HaulingTruckTypeOption.values) {
        expect(option.displayLabel, isNotEmpty);
      }
    });

    test('exposes a Van option, which providers can register', () {
      final labels =
          HaulingTruckTypeOption.values.map((o) => o.displayLabel).toList();
      expect(labels, contains('Van'));
      expect(HaulingTruckTypeOption.van.apiValue, 'van');
    });
  });
}
