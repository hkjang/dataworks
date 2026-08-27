(() => {
  'use strict'

  const header = document.querySelector('[data-header]')
  const menuButton = document.querySelector('[data-menu-button]')
  const navigation = document.querySelector('[data-navigation]')
  const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches

  const setMenu = (open) => {
    if (!menuButton || !navigation) return
    menuButton.setAttribute('aria-expanded', String(open))
    navigation.classList.toggle('is-open', open)
    document.body.classList.toggle('is-menu-open', open)
    const label = menuButton.querySelector('.sr-only')
    if (label) label.textContent = open
      ? (document.documentElement.lang === 'ko' ? '메뉴 닫기' : 'Close menu')
      : (document.documentElement.lang === 'ko' ? '메뉴 열기' : 'Open menu')
  }

  menuButton?.addEventListener('click', () => {
    setMenu(menuButton.getAttribute('aria-expanded') !== 'true')
  })

  navigation?.querySelectorAll('a').forEach((link) => {
    link.addEventListener('click', () => setMenu(false))
  })

  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') setMenu(false)
  })

  document.addEventListener('click', (event) => {
    if (!navigation?.classList.contains('is-open')) return
    if (!navigation.contains(event.target) && !menuButton?.contains(event.target)) setMenu(false)
  })

  window.addEventListener('resize', () => {
    if (window.innerWidth > 1050) setMenu(false)
  }, { passive: true })

  const syncHeader = () => header?.classList.toggle('is-scrolled', window.scrollY > 12)
  syncHeader()
  window.addEventListener('scroll', syncHeader, { passive: true })

  document.querySelectorAll('[data-gallery] figure[data-shot]').forEach((figure) => {
    const shot = figure.dataset.shot
    const label = figure.dataset.label || 'Data Works'
    if (!shot) return

    const picture = document.createElement('picture')
    const mobile = document.createElement('source')
    mobile.media = '(max-width: 640px)'
    mobile.srcset = `assets/screenshots/mobile/${shot}.jpg`
    const image = document.createElement('img')
    image.src = `assets/screenshots/desktop/${shot}.jpg`
    image.alt = label
    image.width = 1440
    image.height = 900
    image.loading = 'lazy'
    image.decoding = 'async'
    picture.append(mobile, image)

    const caption = document.createElement('figcaption')
    caption.append(document.createTextNode(label))
    if (figure.dataset.future === 'true') {
      const badge = document.createElement('span')
      badge.textContent = document.documentElement.lang === 'ko' ? '확장 안내' : 'Extension preview'
      caption.append(badge)
    }
    figure.append(picture, caption)
  })

  const dialog = document.querySelector('[data-lightbox-dialog]')
  const dialogImage = dialog?.querySelector('img')
  const dialogCaption = dialog?.querySelector('p')
  let lightboxTrigger = null

  const openLightbox = (figure) => {
    const sourceImage = figure.querySelector('img')
    if (!dialog || !dialogImage || !sourceImage) return
    lightboxTrigger = figure
    dialogImage.src = sourceImage.currentSrc || sourceImage.src
    dialogImage.alt = sourceImage.alt
    if (dialogCaption) dialogCaption.textContent = figure.querySelector('figcaption')?.textContent?.trim() || sourceImage.alt
    if (typeof dialog.showModal === 'function') dialog.showModal()
  }

  const closeLightbox = () => {
    if (!dialog?.open) return
    dialog.close()
  }

  document.querySelectorAll('[data-lightbox]').forEach((figure) => {
    figure.tabIndex = 0
    figure.setAttribute('role', 'button')
    figure.setAttribute('aria-label', `${figure.querySelector('img')?.alt || figure.dataset.label || 'Data Works'} — ${document.documentElement.lang === 'ko' ? '크게 보기' : 'enlarge'}`)
    figure.addEventListener('click', () => openLightbox(figure))
    figure.addEventListener('keydown', (event) => {
      if (event.key === 'Enter' || event.key === ' ') {
        event.preventDefault()
        openLightbox(figure)
      }
    })
  })

  dialog?.querySelector('[data-lightbox-close]')?.addEventListener('click', closeLightbox)
  dialog?.addEventListener('click', (event) => {
    if (event.target === dialog) closeLightbox()
  })
  dialog?.addEventListener('close', () => {
    dialogImage?.removeAttribute('src')
    lightboxTrigger?.focus()
    lightboxTrigger = null
  })

  document.querySelectorAll('.faq-list details').forEach((details) => {
    details.addEventListener('toggle', () => {
      if (!details.open) return
      document.querySelectorAll('.faq-list details[open]').forEach((other) => {
        if (other !== details) other.open = false
      })
    })
  })

  const revealItems = document.querySelectorAll('.reveal')
  if (reducedMotion || !('IntersectionObserver' in window)) {
    revealItems.forEach((item) => item.classList.add('is-visible'))
  } else {
    const observer = new IntersectionObserver((entries) => {
      entries.forEach((entry) => {
        if (!entry.isIntersecting) return
        entry.target.classList.add('is-visible')
        observer.unobserve(entry.target)
      })
    }, { rootMargin: '0px 0px -7% 0px', threshold: 0.08 })
    revealItems.forEach((item) => observer.observe(item))
  }
})()
