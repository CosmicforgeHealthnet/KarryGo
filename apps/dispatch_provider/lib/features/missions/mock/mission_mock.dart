import '../data/mission_model.dart';

/// Mock data for the Multi-Stop Commerce Mission (design board 09).
/// Delete when the backend supports multi-stop contracts.
class MissionMock {
  MissionMock._();

  // ── Active mission (state 01 seed) ────────────────────────────────
  //
  // A 4-stop, 28-parcel commerce mission. First stop is already in
  // progress with 0 of 3 parcels scanned, so the flow starts at "scan
  // parcel" — matching state 06/07 of the board.
  static Mission activeMission() {
    final now = DateTime.now();
    return Mission(
      id: 'CM-284731',
      sourceOrgName: 'Karryam Commerce',
      createdAt: now.subtract(const Duration(minutes: 12)),
      startedAt: now.subtract(const Duration(minutes: 5)),
      status: MissionStatus.inProgress,
      totalDistanceKm: 62.4,
      stops: [
        MissionStop(
          id: 'stop-1',
          number: 1,
          name: 'Ikeja City Mall',
          address: '12 Chardonnay Avenue Way, Ikeja, Lagos',
          lat: 6.5955,
          lng: 3.3495,
          distanceFromPreviousKm: 2.4,
          etaMinutes: 8,
          arriveBy: now.add(const Duration(minutes: 30)),
          contactName: 'Karryam Store',
          contactPhone: '+234 700 KARRYAM',
          status: MissionStopStatus.pending,
          credentialCode: '742961',
          credentialExpiresAt: now.add(const Duration(minutes: 10)),
          parcels: const [
            MissionParcel(
              id: 'PK-782451',
              handling: ParcelHandling.standard,
              weightKg: 2.4,
              itemType: 'Standard parcel',
              barcode: 'PK-782451',
            ),
            MissionParcel(
              id: 'PK-782452',
              handling: ParcelHandling.fragile,
              weightKg: 1.1,
              itemType: 'Electronics',
              barcode: 'PK-782452',
            ),
            MissionParcel(
              id: 'PK-782453',
              handling: ParcelHandling.chilled,
              weightKg: 3.6,
              itemType: 'Pharmaceuticals',
              barcode: 'PK-782453',
            ),
          ],
        ),
        MissionStop(
          id: 'stop-2',
          number: 2,
          name: 'Surulere Market',
          address: 'Surulere, Lagos',
          lat: 6.5015,
          lng: 3.3480,
          distanceFromPreviousKm: 16.1,
          etaMinutes: 28,
          arriveBy: now.add(const Duration(hours: 1, minutes: 15)),
          contactName: 'Surulere Depot',
          contactPhone: '+234 700 SURULERE',
          status: MissionStopStatus.pending,
          parcels: List.generate(
            18,
            (i) => MissionParcel(
              id: 'PK-${790000 + i}',
              handling: i % 4 == 0
                  ? ParcelHandling.fragile
                  : ParcelHandling.standard,
              weightKg: 1.0 + (i % 5),
              itemType: i % 4 == 0 ? 'Retail goods' : 'Standard parcel',
            ),
          ),
        ),
        MissionStop(
          id: 'stop-3',
          number: 3,
          name: 'Yaba Tech Hub',
          address: 'Yaba, Lagos',
          lat: 6.5090,
          lng: 3.3710,
          distanceFromPreviousKm: 4.8,
          etaMinutes: 12,
          arriveBy: now.add(const Duration(hours: 1, minutes: 45)),
          contactName: 'Yaba Tech Hub',
          contactPhone: '+234 700 YABA',
          status: MissionStopStatus.pending,
          parcels: List.generate(
            7,
            (i) => MissionParcel(
              id: 'PK-${800000 + i}',
              handling: ParcelHandling.documents,
              weightKg: 0.5 + (i % 3) * 0.3,
              itemType: 'Documents',
            ),
          ),
        ),
        MissionStop(
          id: 'stop-4',
          number: 4,
          name: 'Victoria Island',
          address: 'Victoria Island, Lagos',
          lat: 6.4280,
          lng: 3.4210,
          distanceFromPreviousKm: 39.1,
          etaMinutes: 55,
          arriveBy: now.add(const Duration(hours: 3)),
          contactName: 'VI Distribution',
          contactPhone: '+234 700 VI',
          status: MissionStopStatus.pending,
          parcels: List.generate(
            10,
            (i) => MissionParcel(
              id: 'PK-${810000 + i}',
              handling: i % 3 == 0
                  ? ParcelHandling.chilled
                  : ParcelHandling.standard,
              weightKg: 1.5 + (i % 4),
              itemType: 'Mixed goods',
            ),
          ),
        ),
      ],
    );
  }

  // ── Completed mission (state 12 seed) ─────────────────────────────
  static Mission completedMission() {
    final now = DateTime.now();
    return Mission(
      id: 'CM-284720',
      sourceOrgName: 'Tunde Foods Ltd',
      createdAt: now.subtract(const Duration(hours: 6)),
      startedAt: now.subtract(const Duration(hours: 5, minutes: 30)),
      completedAt: now.subtract(const Duration(hours: 1, minutes: 20)),
      status: MissionStatus.completed,
      totalDistanceKm: 48.2,
      stops: const [
        MissionStop(
          id: 'done-1',
          number: 1,
          name: 'Ikeja City Mall',
          address: 'Ikeja, Lagos',
          lat: 6.5955,
          lng: 3.3495,
          parcels: [
            MissionParcel(
              id: 'PK-990001',
              handling: ParcelHandling.standard,
              weightKg: 2.0,
              status: ParcelStatus.delivered,
            ),
            MissionParcel(
              id: 'PK-990002',
              handling: ParcelHandling.fragile,
              weightKg: 1.5,
              status: ParcelStatus.delivered,
            ),
          ],
          status: MissionStopStatus.completed,
          evidenceCaptured: true,
          credentialVerified: true,
        ),
        MissionStop(
          id: 'done-2',
          number: 2,
          name: 'Lekki Phase 1',
          address: 'Lekki, Lagos',
          lat: 6.4410,
          lng: 3.4730,
          parcels: [
            MissionParcel(
              id: 'PK-990003',
              handling: ParcelHandling.chilled,
              weightKg: 3.0,
              status: ParcelStatus.delivered,
            ),
          ],
          status: MissionStopStatus.completed,
          evidenceCaptured: true,
          credentialVerified: true,
        ),
      ],
    );
  }

  // ── Unassigned parcel (state 08 error) ────────────────────────────
  //
  // Used by the scan-conflict screen to show "parcel not in manifest".
  static const unassignedParcel = MissionParcel(
    id: 'PK-992134',
    handling: ParcelHandling.standard,
    weightKg: 2.0,
    itemType: 'Unassigned parcel',
    barcode: 'PK-992134',
  );
}