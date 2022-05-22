import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import ReactJson from 'react-json-view';
import UserConfig from '../core/user.js';

const defaultTheme = {
    scheme: 'rjv-default',
    author: 'mac gainor',

    //transparent main background
    base00: 'rgba(0, 0, 0, 0)',
    base01: 'rgb(245, 245, 245)',
    base02: 'rgb(235, 235, 235)',
    base03: '#93a1a1',
    base04: 'rgba(0, 0, 0, 0.3)',
    base05: '#586e75',
    base06: '#073642',
    base07: '#002b36',
    base08: '#d33682',
    base09: '#cb4b16',
    base0A: '#dc322f',
    base0B: '#859900',
    base0C: '#6c71c4',
    base0D: '#586e75',
    base0E: '#2aa198',
    base0F: '#268bd2'
};


const darkTheme = {
    scheme: 'rjv-default',
    author: 'mac gainor',

    //transparent main background
    base00: 'rgba(0, 0, 0, 0)',
    base01: 'rgb(245, 245, 245)',
    base02: 'rgb(235, 235, 235)',
    base03: '#93a1a1',
    base04: 'rgba(0, 0, 0, 0.3)',
    base05: '#586e75',
    base06: '#073642',
    base07: '#f0fbf6',
    base08: '#d33682',
    base09: '#cbcbc6',
    base0A: '#dc322f',
    base0B: '#859900',
    base0C: '#6c71c4',
    base0D: '#586e75',
    base0E: '#8aa198',
    base0F: '#268bd2'
};

export default class VeloValueRenderer extends React.Component {
    static contextType = UserConfig;
    static propTypes = {
        value: PropTypes.any,
    };

    getTheme = ()=> {
        let theme = this.context.traits &&
            this.context.traits.theme;

        switch (theme) {
        case "github-dimmed-dark":
        case "veloci-dark":
        case "coolgray-dark":
            return darkTheme;

        default:
            return defaultTheme;
        }
    }

    render() {
        let v = this.props.value;

        if (_.isString(v)) {
            return <>{v}</>;
        }

        if (_.isNumber(v)) {
            return JSON.stringify(v);
        }

        if (_.isNull(v)) {
            return "";
        }

        let theme = this.getTheme();

        return (
            <ReactJson name={false}
                       collapsed={1}
                       theme={theme}
                       enableClipboard={false}
                       collapseStringsAfterLength={100}
                       displayObjectSize={false}
                       displayDataTypes={false}
                       src={v} />
        );
    }
};
