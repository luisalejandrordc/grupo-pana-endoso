import { useRef, useState, type FormEvent } from "react";
import { api } from "../../shared/api/client";
import { roleLabel } from "../../shared/lib/roles";
import { useForgetPhoto } from "../../shared/photos/usePhoto";
import { Avatar } from "../../shared/ui/Avatar";
import { type EmployeeProfile } from "../../shared/types";
import { Field } from "../../shared/ui/Field";
import { Icon } from "../../shared/ui/Icon";
import { type Toast } from "../../shared/ui/toast";

const MAX_PHOTO = 2 * 1024 * 1024;

export function ProfilePage({
  user,
  onSignOut,
  onPhotoChanged,
  toast,
}: {
  user: EmployeeProfile;
  onSignOut: () => void;
  /** Called after the picture was set or removed, so the rest of the app shows the new one. */
  onPhotoChanged: () => void;
  toast: (toast: Toast) => void;
}) {
  const fileInput = useRef<HTMLInputElement>(null);
  const [photoBusy, setPhotoBusy] = useState(false);
  const forget = useForgetPhoto();
  const name = `${user.firstName} ${user.lastName}`;

  async function choosePhoto(file: File | undefined) {
    if (!file) return;
    if (!["image/jpeg", "image/png", "image/webp"].includes(file.type))
      return toast({
        title: "Formato no admitido",
        message: "Usa una imagen JPEG, PNG o WebP.",
        tone: "error",
      });
    if (file.size > MAX_PHOTO)
      return toast({
        title: "La foto es muy pesada",
        message: "El máximo es 2 MB.",
        tone: "error",
      });
    setPhotoBusy(true);
    try {
      await api.setMyPhoto(file);
      await forget(user.id);
      onPhotoChanged();
      toast({ title: "Foto actualizada", tone: "success" });
    } catch (error) {
      toast({
        title: "No se pudo guardar la foto",
        message:
          error instanceof Error ? error.message : "Inténtalo nuevamente.",
        tone: "error",
      });
    } finally {
      setPhotoBusy(false);
    }
  }

  async function removePhoto() {
    setPhotoBusy(true);
    try {
      await api.removeMyPhoto();
      await forget(user.id);
      onPhotoChanged();
      toast({ title: "Foto eliminada", tone: "success" });
    } catch (error) {
      toast({
        title: "No se pudo quitar la foto",
        message:
          error instanceof Error ? error.message : "Inténtalo nuevamente.",
        tone: "error",
      });
    } finally {
      setPhotoBusy(false);
    }
  }

  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [saving, setSaving] = useState(false);
  async function submitPassword(event: FormEvent) {
    event.preventDefault();
    if (newPassword.length < 10)
      return toast({
        title: "Contraseña muy corta",
        message: "Utiliza al menos 10 caracteres.",
        tone: "error",
      });
    if (newPassword !== confirmPassword)
      return toast({ title: "Las contraseñas no coinciden", tone: "error" });
    setSaving(true);
    try {
      await api.changePassword({ currentPassword, newPassword });
      toast({
        title: "Contraseña actualizada",
        message: "La sesión finalizará por seguridad.",
        tone: "success",
      });
      onSignOut();
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
    } catch (error) {
      toast({
        title: "No se pudo cambiar la contraseña",
        message:
          error instanceof Error ? error.message : "Inténtalo nuevamente.",
        tone: "error",
      });
    } finally {
      setSaving(false);
    }
  }
  return (
    <div className="stack profile-page">
      <header className="page-head">
        <div>
          <h1>Mi perfil</h1>
          <p>Tus datos corporativos, tu foto y tu acceso.</p>
        </div>
      </header>
      <section className="panel profile-card">
        <div className="profile-identity">
          <Avatar id={user.id} name={name} hasPhoto={user.hasPhoto} size={96} />
          <div className="profile-name">
            <h2>{name}</h2>
            <p>{user.businessEmail}</p>
            <span className="badge badge-neutral">{roleLabel[user.role]}</span>
          </div>
          <div className="profile-photo-actions">
            <input
              ref={fileInput}
              type="file"
              hidden
              accept="image/jpeg,image/png,image/webp"
              onChange={(event) => {
                const file = event.target.files?.[0];
                event.target.value = "";
                void choosePhoto(file);
              }}
            />
            <button
              className="button button-secondary"
              disabled={photoBusy}
              onClick={() => fileInput.current?.click()}
            >
              <Icon name="upload" size={16} />{" "}
              {user.hasPhoto ? "Cambiar foto" : "Subir foto"}
            </button>
            {user.hasPhoto && (
              <button
                className="button button-ghost"
                disabled={photoBusy}
                onClick={removePhoto}
              >
                Quitar foto
              </button>
            )}
            <small>JPEG, PNG o WebP · máximo 2 MB</small>
          </div>
        </div>
        <dl className="profile-data">
          <div>
            <dt>Documento de identidad</dt>
            <dd>
              {user.documentType} {user.documentNumber}
            </dd>
          </div>
          <div>
            <dt>Correo corporativo</dt>
            <dd>{user.businessEmail}</dd>
          </div>
          <div>
            <dt>Rol del sistema</dt>
            <dd>{roleLabel[user.role]}</dd>
          </div>
        </dl>
      </section>
      <section className="workspace-card password-card">
        <div className="password-card-intro">
          <div className="password-icon">
            <Icon name="lock" size={21} />
          </div>
          <div>
            <h3>Cambiar contraseña</h3>
            <p>
              Te recomendamos utilizar una contraseña única de al menos 10
              caracteres.
            </p>
          </div>
        </div>
        <form onSubmit={submitPassword} className="password-change-form">
          <Field label="Contraseña actual">
            <input
              type="password"
              value={currentPassword}
              onChange={(event) => setCurrentPassword(event.target.value)}
              required
              autoComplete="current-password"
            />
          </Field>
          <div className="password-change-divider" />
          <Field label="Nueva contraseña" hint="Mínimo 10 caracteres.">
            <input
              type="password"
              value={newPassword}
              onChange={(event) => setNewPassword(event.target.value)}
              required
              minLength={10}
              autoComplete="new-password"
            />
          </Field>
          <Field label="Confirmar nueva contraseña">
            <input
              type="password"
              value={confirmPassword}
              onChange={(event) => setConfirmPassword(event.target.value)}
              required
              minLength={10}
              autoComplete="new-password"
            />
          </Field>
          <div className="password-rules">
            <span>
              <Icon
                name={newPassword.length >= 10 ? "check-circle" : "circle"}
                size={14}
              />{" "}
              Al menos 10 caracteres
            </span>
            <span>
              <Icon
                name={
                  newPassword && confirmPassword === newPassword
                    ? "check-circle"
                    : "circle"
                }
                size={14}
              />{" "}
              Las contraseñas coinciden
            </span>
          </div>
          <div className="password-form-footer">
            <span>
              <Icon name="shield-check" size={15} /> Al cambiarla, se cerrarán
              las sesiones activas.
            </span>
            <button className="button button-primary" disabled={saving}>
              {saving ? "Guardando…" : "Actualizar contraseña"}{" "}
              <Icon name="arrow-right" size={15} />
            </button>
          </div>
        </form>
      </section>
      <div className="profile-signout-row">
        <div>
          <strong>¿Terminaste por hoy?</strong>
          <span>Cierra tu sesión de forma segura en este dispositivo.</span>
        </div>
        <button className="button button-secondary" onClick={onSignOut}>
          <Icon name="logout" size={16} /> Cerrar sesión
        </button>
      </div>
    </div>
  );
}

export function ProfileData({
  label,
  value,
}: {
  label: string;
  value: string;
}) {
  return (
    <div className="profile-data-item">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}
