package apps.mediaplayer;

import danger.audio.AudioManager;
import danger.audio.PlayableAudio;

/**
 * SonicBOOM's playable for a song, as on a real device. In the simulator the app returns a
 * silent SimPlayable instead; the host overrides MediaPlayer.playableForMusicFile to call this,
 * so songs load as Streams (danger.audio.WebStream) and play through Web Audio.
 */
final class WebPlayables {
    static PlayableAudio forFile(MusicFile mf) {
        PlayableAudio playable = AudioManager.load(mf.file(), false);
        if (playable != null) playable.setVolumeClass(MediaPlayer.sVolumeManagerID);
        return playable;
    }
}
