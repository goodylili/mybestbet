import styles from "./mybestbet-wordmark.module.css"

/**
 * The lowercase "mybestbet" wordmark. Size and colour follow the theme by
 * default; a className passed in always wins, since the base rule has no
 * specificity.
 */
export function MybestbetWordmark({ className = "" }: { className?: string }) {
  return <span className={styles.wm + (className ? " " + className : "")} aria-hidden="true" />
}
