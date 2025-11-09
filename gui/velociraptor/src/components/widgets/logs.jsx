import _ from 'lodash';
import "./logs.css";

import React, { Component } from 'react';
import PropTypes from 'prop-types';

const tags = new RegExp("(.*?)<(green|red|yellow)>(.+?)(</.*?>|$)(.*)", "g");


export default class VeloLog extends Component {
    static propTypes = {
        value: PropTypes.any,
    }

    render() {

        let value = "";
        if (_.isString(this.props.value)) {
            value = this.props.value;
        }

        let matches = [...value.matchAll(tags)];
        if (_.isEmpty(matches)) {
            return <>{value}</>;
        }

        return _.map(matches, (x, i)=>{
            return <React.Fragment key={i}>
                     <span>{ x[1] }</span>
                     <span className={ "log-" + x[2] }>
                       {x[3]}
                     </span>
                     <span>{ x[5] }</span>
                   </React.Fragment>;
        });
    }
}
