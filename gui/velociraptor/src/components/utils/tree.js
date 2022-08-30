import PropTypes from 'prop-types';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';

const text_color = "#8f8f8f";
const background_color = '#f5f5f5';
const active_background_color = "#dee0ff";

export const Header = ({onSelect, style, customStyles, node}) => {
    return (
        <div style={style.base} onClick={onSelect}>
          <div className="tree-folder">
            {node.toggled ? <FontAwesomeIcon icon="folder-open" /> : <FontAwesomeIcon icon="folder" /> }
            {node.name}
          </div>
        </div>
    );
};

Header.propTypes = {
    onSelect: PropTypes.func,
    node: PropTypes.object,
    style: PropTypes.object,
    customStyles: PropTypes.object
};

Header.defaultProps = {
    customStyles: {}
};


const theme_template = ()=>{return {
    tree: {
        base: {
            listStyle: 'none',
            backgroundColor: background_color,
            margin: 0,
            padding: 0,
            color: text_color,
            marginLeft: '-20px',
            marginTop: '-20px',
        },
        node: {
            base: {
                position: 'relative'
            },
            link: {
                cursor: 'pointer',
                position: 'relative',
                padding: '0px 5px',
                display: 'block'
            },
            activeLink: {
                background: active_background_color,
            },
            toggle: {
                base: {
                    position: 'relative',
                    display: 'none',
                    verticalAlign: 'top',
                    marginLeft: '-5px',
                    height: '24px',
                    width: '24px'
                },
                wrapper: {
                    position: 'absolute',
                    top: '20%',
                    left: '50%',
                    margin: '-7px 0 0 -7px',
                    height: '9px'
                },
                height: 9,
                width: 9,
                arrow: {
                    fill: text_color,
                    strokeWidth: 0
                }
            },
            header: {
                base: {
                    display: 'inline-block',
                    verticalAlign: 'top',
                    color: "black"
                },
                connector: {
                    width: '2px',
                    height: '12px',
                    borderLeft: 'solid 2px black',
                    borderBottom: 'solid 2px black',
                    position: 'absolute',
                    top: '0px',
                    left: '-21px'
                },
                title: {
                    lineHeight: '24px',
                    verticalAlign: 'middle'
                }
            },
            subtree: {
                listStyle: 'none',
                paddingLeft: '19px'
            },
            loading: {
                color: '#E2C089'
            }
        }
    }
}};


export const getTheme = (theme)=> {
    let result = theme_template();

    switch (theme) {
    case "github-dimmed-dark":
    case "veloci-dark":
    case "ncurses":
    case "coolgray-dark":
        result.tree.base.backgroundColor = '#030303';
        result.tree.base.color = '#010101';
        result.tree.node.activeLink.background = '#202020';
        result.tree.node.header.base.color = '#020202';
        return result;

    default:
        return result;
    }
};
