import 'package:latlong2/latlong.dart';
List<LatLng> decodeRoutePolyline(String? encoded) {
  if (encoded == null || encoded.isEmpty || encoded.length > 200000) return const [];
  final points=<LatLng>[];
  var index=0,lat=0,lng=0;
  int read() {
    var value=0,shift=0;
    while(true) {
      if(index>=encoded.length || shift>30) throw const FormatException('Invalid polyline');
      final byte=encoded.codeUnitAt(index++)-63;
      if(byte<0 || byte>63) throw const FormatException('Invalid polyline');
      value|=(byte&31)<<shift;
      if(byte<32) break;
      shift+=5;
    }
    return value.isOdd ? ~(value>>1) : value>>1;
  }
  try {
    while(index<encoded.length) {
      lat+=read();lng+=read();
      if(lat.abs()>9000000 || lng.abs()>18000000) return const [];
      points.add(LatLng(lat/1e5,lng/1e5));
    }
  } on FormatException { return const []; }
  return points.length<2 ? const [] : points;
}
