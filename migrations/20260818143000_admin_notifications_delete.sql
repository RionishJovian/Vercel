-- Recipients can delete their own in-app notifications (read/acknowledged inbox cleanup).
GRANT DELETE ON public.admin_notifications TO authenticated;

CREATE POLICY "recipient can delete own notifications"
  ON public.admin_notifications
  FOR DELETE TO authenticated
  USING (recipient_id = auth.uid());
