import 'package:flutter/foundation.dart';

import '../data/mission_model.dart';
import '../mock/mission_mock.dart';

/// Local-only mission state. Wraps [MissionMock] so the UI can advance
/// stops and parcels without a backend. Swap the internals for real API
/// calls when the multi-stop contract lands.
class MissionController extends ChangeNotifier {
  MissionController({Mission? initial})
      : _mission = initial ?? MissionMock.activeMission();

  Mission _mission;
  Mission? get mission => _mission;
  bool get hasMission => _mission.status.isActive;

  // ── Stop navigation ──────────────────────────────────────────────

  MissionStop? get currentStop => _mission.currentStop;
  MissionStop? get nextStop => _mission.nextStop;

  int get currentStopNumber {
    final stop = currentStop;
    return stop?.number ?? _mission.totalStops;
  }

  // ── Stop actions ─────────────────────────────────────────────────

  /// Marks the current stop as arrived. Called from state 04.
  void markArrived() {
    final i = _mission.currentStopIndex;
    if (i < 0) return;
    final stop = _mission.stops[i];
    if (stop.status == MissionStopStatus.arrived) return;

    _replaceStop(
      i,
      stop.copyWith(
        status: MissionStopStatus.arrived,
        arrivedAt: DateTime.now(),
      ),
    );
    notifyListeners();
  }

  /// Marks the current stop as processing (rider began scanning).
  void beginProcessing() {
    final i = _mission.currentStopIndex;
    if (i < 0) return;
    final stop = _mission.stops[i];
    if (stop.status == MissionStopStatus.processing) return;

    _replaceStop(
      i,
      stop.copyWith(status: MissionStopStatus.processing),
    );
    notifyListeners();
  }

  /// Scans a parcel at the current stop. Returns true if the parcel was
  /// found in the manifest (and marked scanned). Returns false if the
  /// barcode doesn't belong to this stop (state 08 error).
  bool scanParcel(String barcode) {
    final i = _mission.currentStopIndex;
    if (i < 0) return false;
    final stop = _mission.stops[i];
    final idx = stop.parcels.indexWhere((p) => p.barcode == barcode);
    if (idx < 0) return false;

    final updated = [...stop.parcels];
    updated[idx] = updated[idx].copyWith(status: ParcelStatus.scanned);
    _replaceStop(i, stop.copyWith(parcels: updated));
    notifyListeners();
    return true;
  }

  /// Marks the credential verified (state 09).
  void verifyCredential() {
    final i = _mission.currentStopIndex;
    if (i < 0) return;
    final stop = _mission.stops[i];
    _replaceStop(i, stop.copyWith(credentialVerified: true));
    notifyListeners();
  }

  /// Marks evidence captured (state 10).
  void markEvidenceCaptured({
    List<String>? photos,
    String? signatureUrl,
    String? notes,
  }) {
    final i = _mission.currentStopIndex;
    if (i < 0) return;
    final stop = _mission.stops[i];
    _replaceStop(
      i,
      stop.copyWith(
        evidencePhotos: photos ?? stop.evidencePhotos,
        evidenceSignatureUrl: signatureUrl ?? stop.evidenceSignatureUrl,
        evidenceNotes: notes ?? stop.evidenceNotes,
        evidenceCaptured: true,
      ),
    );
    notifyListeners();
  }

  /// Completes the current stop and advances to the next (state 11).
  void completeCurrentStop() {
    final i = _mission.currentStopIndex;
    if (i < 0) return;
    final stop = _mission.stops[i];
    _replaceStop(
      i,
      stop.copyWith(
        status: MissionStopStatus.completed,
        completedAt: DateTime.now(),
      ),
    );

    // If that was the last stop, mark the whole mission complete.
    if (_mission.allStopsComplete) {
      _mission = _mission.copyWith(
        status: MissionStatus.completed,
        completedAt: DateTime.now(),
      );
    }
    notifyListeners();
  }

  // ── Helpers ──────────────────────────────────────────────────────

  void _replaceStop(int index, MissionStop updated) {
    final list = [..._mission.stops];
    list[index] = updated;
    _mission = _mission.copyWith(stops: list);
  }

  /// Resets to a fresh active mission — handy for testing the whole flow
  /// from the start without restarting the app.
  void reset() {
    _mission = MissionMock.activeMission();
    notifyListeners();
  }
}