import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import ReactJson from 'react-json-view';
import UserConfig from '../core/user.jsx';
import VeloTimestamp from "./time.jsx";
import ContextMenu from './context.jsx';

// Try to detect something that looks like a timestamp.
const timestamp_regex = /(\d{4}-[01]\d-[0-3]\dT[0-2]\d:[0-5]\d:[0-5]\d(?:\.\d+)?(?:[+-][0-2]\d:?[0-5]\d|Z))/;

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
        case "coolgray-dark":
            return darkTheme;

        default:
            return defaultTheme;
        }
    }

    // If the cell contains something that looks like a timestamp,
    // format it as such.
    maybeFormatTime = x => {
        let parts = x.split(timestamp_regex);
        return _.map(parts, (x, idx)=>{
            if (timestamp_regex.test(x)) {
                return <VeloTimestamp key={idx} iso={x} />;
            }
            return x;
        });
    }

    render() {
        let v = this.props.value;

        if (_.isString(v)) {
            return <ContextMenu value={v}>{this.maybeFormatTime(v)}</ContextMenu>;
        }

        if (_.isNumber(v)) {
            return JSON.stringify(v);
        }

        if (_.isNull(v)) {
            return "";
        }

        let theme = this.getTheme();

        return (
            <ContextMenu value={v}>
              <ReactJson name={false}
                         collapsed={1}
                         theme={theme}
                         enableClipboard={false}
                         collapseStringsAfterLength={100}
                         displayObjectSize={false}
                         displayDataTypes={false}
                         src={v} />
            </ContextMenu>
        );
    }
}
