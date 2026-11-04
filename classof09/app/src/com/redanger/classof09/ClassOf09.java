package com.redanger.classof09;

import danger.app.Application;
import danger.app.Event;
import danger.app.Timer;

/** Class of '09 for the Sidekick 3. */
public class ClassOf09 extends Application {
	private GameView mView;
	private Timer mTimer;

	public void launch() {
		mTimer = new Timer(50, true, this);
		create();
	}

	/** Builds the game window; logs and retries on the next resume if it fails. */
	private boolean create() {
		if (mView != null) return true;
		try {
			mView = new GameView();
			return true;
		} catch (Throwable t) {
			Log.e("failed to create game window", t);
			return false;
		}
	}

	public void resume() {
		if (mTimer == null) mTimer = new Timer(50, true, this);
		if (!create()) return;
		mView.show();
		mView.resumed();
		mTimer.start();
	}

	public void suspend() {
		if (mTimer != null) mTimer.stop();
		if (mView != null) mView.suspend();
	}

	public void quit() {
		if (mTimer != null) mTimer.stop();
		if (mView != null) mView.quit();
	}

	public boolean receiveEvent(Event e) {
		if (e.type == Event.EVENT_TIMER) {
			if (mView != null) {
				try {
					mView.tick();
				} catch (Throwable t) {
					Log.e("tick failed", t);
					mView.cardLost();
				}
			}
			return true;
		}
		return super.receiveEvent(e);
	}
}
