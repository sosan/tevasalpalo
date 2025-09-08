document.addEventListener("DOMContentLoaded", function () {
    loadActivityModal();
});

function loadActivityModal() {
    const dialog = document.getElementById('config-menu');
    const openBtn = document.getElementById('openConfigMenuBtn');
    const closeBtn = document.querySelector('.close-dialog-config-menu');
    
    openBtn.addEventListener('click', () => {
        dialog.showModal();
    });
    
    closeBtn.addEventListener('click', () => {
        dialog.close();
    });
    
    // Opcional: Cerrar haciendo clic fuera del contenido del diálogo (solo showModal)
    dialog.addEventListener('click', (event) => {
        // dialog.close() se puede llamar aquí también, pero hay que tener cuidado
        // con cerrarlo al hacer clic dentro. Una forma más robusta es comprobar el target.
        if (event.target === dialog) {
        dialog.close();
        }
    });
}
