import React from 'react';
import PropTypes from 'prop-types';
import { getPagination } from './utils/getPagination';
import PageItem from './PageItem';

export default class Pagination extends React.Component {

    render() {

        const {
            onClick,
            size,
            ariaLabel,
            activeBgColor,
            activeBorderColor,
            disabledBgColor,
            disabledBorderColor,
            bgColor,
            borderColor,
            activeColor,
            color,
            disabledColor,
            circle,
            shadow,
            center,
            className
        } = this.props;
        const pagination = getPagination(this.props);
        return (
            <nav
                aria-label={ariaLabel}
                className={`row ${center && 'justify-content-center'} ${className && className}`}>
                <ul
                    style={shadowStyle(shadow, circle)}
                    className={`pagination ${(size === 'sm' || size === 'lg') && 'pagination-' + size}`}>
                    {
                        pagination.map((page, i) =>
                            <PageItem
                                key={`${page}-${i}`}
                                text={page.text}
                                page={page.page}
                                className={page.class}
                                href={page.href}
                                onClick={onClick}
                                activeBgColor={activeBgColor}
                                activeBorderColor={activeBorderColor}
                                disabledBgColor={disabledBgColor}
                                disabledBorderColor={disabledBorderColor}
                                bgColor={bgColor}
                                borderColor={borderColor}
                                activeColor={activeColor}
                                color={color}
                                disabledColor={disabledColor}
                                circle={circle}
                                shadow={shadow}
                                size={size} />
                        )
                    }
                </ul>
            </nav>

        );
    }
}

Pagination.propTypes = {
    totalPages: PropTypes.number.isRequired,
    currentPage: PropTypes.number.isRequired,
    ariaLabel: PropTypes.string,
    size: PropTypes.string,
    showMax: PropTypes.number,
    activeClass: PropTypes.string,
    defaultClass: PropTypes.string,
    disabledClass: PropTypes.string,
    threeDots: PropTypes.bool,
    href: PropTypes.string,
    pageOneHref: PropTypes.string,
    prevNext: PropTypes.bool,
    prevText: PropTypes.string,
    nextText: PropTypes.string,
    center: PropTypes.bool,
    onClick: PropTypes.func,
    activeBgColor: PropTypes.string,
    activeBorderColor: PropTypes.string,
    disabledBgColor: PropTypes.string,
    disabledBorderColor: PropTypes.string,
    bgColor: PropTypes.string,
    borderColor: PropTypes.string,
    activeColor: PropTypes.string,
    disabledColor: PropTypes.string,
    color: PropTypes.string,
    circle: PropTypes.bool,
    shadow: PropTypes.bool,
    className: PropTypes.string
};

Pagination.defaultProps = {
    currentPage: 1,
    ariaLabel: 'Page navigator',
    activeClass: 'active',
    disabledClass: 'disabled',
    showMax: 5,
    center: false,
    size: 'md', // sm md lg
    prevNext: true,
    prevText: '⟨',
    nextText: '⟩',
    circle: false,
    shadow: false,
}

const shadowStyle = (showShadow, isCircle) => {
    if (!showShadow) return {}
    if (isCircle) return {}
    return {
        WebkitBoxShadow: '0 8px 17px 0 rgba(0,0,0,0.2),0 6px 20px 0 rgba(0,0,0,0.19)',
        boxShadow: '0px 8px 17px 0px rgba(0,0,0,0.2),0px 6px 20px 0px rgba(0,0,0,0.19)'
    }
}
