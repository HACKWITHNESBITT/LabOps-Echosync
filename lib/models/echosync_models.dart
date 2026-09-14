/// Wire models mirrored from the EchoSync Go backend (internal/model).
library;

class RadarLink {
  final List<double> from;
  final List<double> to;

  const RadarLink({required this.from, required this.to});

  factory RadarLink.fromJson(Map<String, dynamic> json) {
    final link = (json['link'] as List?) ?? const [];
    return RadarLink(
      from: _vec(link.isNotEmpty ? link[0] : null),
      to: _vec(link.length > 1 ? link[1] : null),
    );
  }

  static List<double> _vec(dynamic v) {
    if (v is List) {
      return v.map((e) => (e as num).toDouble()).toList();
    }
    return const [0, 0, 0];
  }
}

class RadarTarget {
  final String id;
  final double distanceM;
  final double bearingDeg;
  final List<String> sharedTokens;
  final double similarity;
  final RadarLink link;

  const RadarTarget({
    required this.id,
    required this.distanceM,
    required this.bearingDeg,
    required this.sharedTokens,
    required this.similarity,
    required this.link,
  });

  factory RadarTarget.fromJson(Map<String, dynamic> json) => RadarTarget(
        id: json['id'] ?? '',
        distanceM: (json['distance_m'] as num?)?.toDouble() ?? 0,
        bearingDeg: (json['bearing_deg'] as num?)?.toDouble() ?? 0,
        sharedTokens: (json['shared_tokens'] as List?)?.cast<String>() ?? const [],
        similarity: (json['similarity'] as num?)?.toDouble() ?? 0,
        link: RadarLink.fromJson(json),
      );
}

class RadarState {
  final List<double> self;
  final List<RadarTarget> targets;
  final double radiusM;
  final double sweepHz;

  const RadarState({
    required this.self,
    required this.targets,
    required this.radiusM,
    required this.sweepHz,
  });

  factory RadarState.fromJson(Map<String, dynamic> json) => RadarState(
        self: RadarLink._vec(json['self']),
        targets: (json['targets'] as List? ?? const [])
            .map((e) => RadarTarget.fromJson(e))
            .toList(),
        radiusM: (json['radius_m'] as num?)?.toDouble() ?? 15,
        sweepHz: (json['sweep_hz'] as num?)?.toDouble() ?? 0.25,
      );
}

class EchoMatch {
  final String id;
  final String peerUserId;
  final List<String> sharedTokens;
  final double similarity;
  final double distanceM;
  final String icebreaker;
  final DateTime createdAt;
  final DateTime expiresAt;
  final int ttlSeconds;

  const EchoMatch({
    required this.id,
    required this.peerUserId,
    required this.sharedTokens,
    required this.similarity,
    required this.distanceM,
    required this.icebreaker,
    required this.createdAt,
    required this.expiresAt,
    required this.ttlSeconds,
  });

  factory EchoMatch.fromJson(Map<String, dynamic> json) => EchoMatch(
        id: json['id'] ?? '',
        peerUserId: json['peer_user_id'] ?? '',
        sharedTokens: (json['shared_tokens'] as List?)?.cast<String>() ?? const [],
        similarity: (json['similarity'] as num?)?.toDouble() ?? 0,
        distanceM: (json['distance_m'] as num?)?.toDouble() ?? 0,
        icebreaker: json['icebreaker'] ?? '',
        createdAt: DateTime.parse(json['created_at'] ?? DateTime.now().toIso8601String()),
        expiresAt: DateTime.parse(json['expires_at'] ?? DateTime.now().toIso8601String()),
        ttlSeconds: (json['ttl_seconds'] as num?)?.toInt() ?? 0,
      );

  int get remainingSeconds {
    final seconds = expiresAt.difference(DateTime.now()).inSeconds;
    return seconds < 0 ? 0 : seconds;
  }
}

/// Server → client WebSocket frames.
sealed class WsFrame {
  const WsFrame();
}

class HelloFrame extends WsFrame {
  final String message;
  const HelloFrame(this.message);
}

class MatchFrame extends WsFrame {
  final EchoMatch match;
  final int? latencyMs;
  const MatchFrame(this.match, this.latencyMs);
}

class RadarFrame extends WsFrame {
  final RadarState state;
  const RadarFrame(this.state);
}

class PingFrame extends WsFrame {
  const PingFrame();
}

class WsParseError extends WsFrame {
  final String reason;
  const WsParseError(this.reason);
}