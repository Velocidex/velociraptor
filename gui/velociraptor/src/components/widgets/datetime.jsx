import _ from 'lodash';
import './datetime.css';

import React, { Component } from 'react';
import PropTypes from 'prop-types';
import T from '../i8n/i8n.jsx';
import UserConfig from '../core/user.jsx';
import ButtonGroup from 'react-bootstrap/ButtonGroup';
import InputGroup from 'react-bootstrap/InputGroup';
import ToolTip from '../widgets/tooltip.jsx';
import Row from 'react-bootstrap/Row';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import Dropdown from 'react-bootstrap/Dropdown';

import Form from 'react-bootstrap/Form';

import VeloTimestamp, {
    ToStandardTime,
    FormatRFC3339,
} from '../utils/time.jsx';


const day = 1000 * 60 * 60 * 24;
const week = 7 * day;

class Calendar extends Component {
    static contextType = UserConfig;

    static propTypes = {
        // A string in RFC3339 format.
        value: PropTypes.string,

        // Update the timestamp with the RFC3339 string.
        onChange: PropTypes.func.isRequired,
    }

    componentDidMount = () => {
        if(this.props.value) {
            this.setState({
                value: this.props.value,
                first_day_to_render: this.getFirstDayToRender(this.props.value)});
        }
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (this.props.value !== this.state.value) {
            this.setState({
                value: this.props.value,
                first_day_to_render: this.getFirstDayToRender(this.props.value)});
        }
    }

    setTime = t=>{
        let timezone = this.context.traits.timezone || "UTC";
        this.props.onChange(FormatRFC3339(t, timezone));
    }

    getMonthName = x=>{
        switch (x % 12) {
        case 0: return T("January"); break;
        case 1: return T("February"); break;
        case 2: return T("March"); break;
        case 3: return T("April"); break;
        case 4: return T("May"); break;
        case 5: return T("June"); break;
        case 6: return T("July"); break;
        case 7: return T("August"); break;
        case 8: return T("September"); break;
        case 9: return T("October"); break;
        case 10: return T("November"); break;
        case 11: return T("December"); break;
        default: return "";
        };
    }

    renderDay = (ts, base)=>{
        let classname = "calendar-day";
        let day_of_month = ts.getDate();
        if (day_of_month === base.getDate() &&
            ts.getMonth() === base.getMonth() &&
            ts.getFullYear() == base.getFullYear()) {
            classname = "calendar-selected-date";
        }

        if (ts.getMonth() !== base.getMonth()) {
            classname = "calendar-other-day";
        }

        let str = day_of_month.toString();
        if (day_of_month < 10) {
            str = " " + str;
        }

        return <div
                 onClick={()=>this.setTime(ts)}
                 className={classname}>
                 {str}
               </div>;
    }

    renderWeek = (ts, base)=>{
        let res = [];
        for(let j=0;j<7;j++) {
            let day_ts = new Date();
            day_ts.setTime(ts.getTime() + j * day);

            res.push(<td key={j}>
                       {this.renderDay(day_ts, base)}
                     </td>);
        }
        return res;
    }

    renderMonth = (ts, base)=>{
        let res = [];
        for(let j=0;j<=5;j++) {
            let week_start = new Date();
            let week_start_sec = ts.getTime() + j * week;
            week_start.setTime(week_start_sec);

            res.push(<tr key={j}
                         className="calendar-week">
                       {this.renderWeek(week_start, base)}
                     </tr>);
        }
        return <table className="calendar-month">
                 <tbody>
                   {res}
                 </tbody>
               </table>;
    }

    getFirstDayToRender = t=>{
        let ts = ToStandardTime(t);
        if (!_.isDate(ts)) {
            return undefined;
        }

        let year = ts.getFullYear();
        let prev_month = ts.getMonth() -1;
        if (prev_month < 0) {
            prev_month = 11;
            year -= 1;
        }

        let first_day_of_month = new Date(ts.getTime());
        first_day_of_month.setDate(0);

        let day_of_week = first_day_of_month.getDay();
        let first_day_to_render = new Date();
        first_day_to_render.setTime(first_day_of_month.getTime() - day_of_week * day);

        // Reset the time to the start of the day.
        first_day_to_render.setMilliseconds(0);
        first_day_to_render.setSeconds(0);
        first_day_to_render.setMinutes(0);
        first_day_to_render.setHours(0);

        return first_day_to_render;
    }

    state = {
        value: undefined,
        first_day_to_render: undefined,
    }

    render() {
        // Figure out the start of the month block.
        let ts = ToStandardTime(this.props.value);
        let start = this.state.first_day_to_render;
        if (!_.isDate(ts) || !_.isDate(start)) {
            return <></>;
        }


        return <div className="calendar-selector">
                 <Row>
                   <ButtonGroup>
                     <Button onClick={e=>{
                         let ts = new Date();
                         ts.setTime(start.getTime());
                         ts.setMonth(start.getMonth() - 1);
                         this.setState({first_day_to_render: ts});

                         e.stopPropagation();
                         e.preventDefault();
                         return false;
                     }}>
                       <FontAwesomeIcon icon="backward"/>
                     </Button>
                     <Button>
                       {this.getMonthName(start.getMonth() + 1)}
                     </Button>
                     <Button onClick={e=>{
                         let ts = new Date();
                         ts.setTime(start.getTime());
                         ts.setMonth(start.getMonth() + 1);
                         this.setState({first_day_to_render: ts});

                         e.preventDefault();
                         e.stopPropagation();
                         return false;
                     }}>
                       <FontAwesomeIcon icon="forward"/>
                     </Button>
                   </ButtonGroup>
                 </Row>
                 <Row>
                   {this.renderMonth(start, ts)}
                 </Row>
                 <Row>
                   <ButtonGroup>
                     <Button onClick={e=>{
                         let ts = new Date();
                         ts.setTime(start.getTime());
                         ts.setFullYear(start.getFullYear() - 1);
                         this.setState({first_day_to_render: ts});

                         e.stopPropagation();
                         e.preventDefault();
                         return false;
                     }}>
                       <FontAwesomeIcon icon="backward"/>
                     </Button>
                     <Button>
                       {T("Year") + " " + start.getFullYear().toString()}
                     </Button>
                     <Button onClick={e=>{
                         let ts = new Date();
                         ts.setTime(start.getTime());
                         ts.setFullYear(start.getFullYear() + 1);
                         this.setState({first_day_to_render: ts});

                         e.stopPropagation();
                         e.preventDefault();
                         return false;
                     }}>
                       <FontAwesomeIcon icon="forward"/>
                     </Button>
                   </ButtonGroup>
                 </Row>
               </div>;
    }
}


export default class DateTimePicker extends Component {
    static contextType = UserConfig;

    static propTypes = {
        // A string in RFC3339 format.
        value: PropTypes.string,

        // Update the timestamp with the RFC3339 string.
        onChange: PropTypes.func.isRequired,
    }

    state = {
        // Current edited time - it will be parsed from RFC3339 format
        // when the user clicks the save button.
        value: "",
        invalid: false,
    }

    componentDidMount() {
        this.setTime(this.props.value);
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (this.props.value !== prevProps.value) {
            this.setTime(this.props.value);
        }
    }

    getTime = offset=>{
        let date = new Date();
        date.setTime(date.getTime() + offset);
        return date;
    }

    getMonth = offset=>{
        let date = new Date();
        date.setMonth(date.getMonth() + offset);
        return date;
    }

    setTime = t=>{
        // Make sure the time is valid before setting it but we still
        // set the RFC3339 string.
        if(_.isString(t)) {
            let ts = ToStandardTime(t);
            if (_.isDate(ts)) {
                this.setState({value: t, invalid: false});
                this.props.onChange(t);
            }
        }
    }

    render() {
        let formatted_ts = this.props.value;
        let timezone = this.context.traits.timezone || "UTC";

        let presets = [
            [()=>T("A week from now"), ()=>this.getTime(week)],
            [()=>T("Now"), ()=>this.getTime(0)],
            [()=>T("A day ago"), ()=>this.getTime(- day)],
            [()=>T("A week ago"), ()=>this.getTime(- week)],
            [()=>T("A month ago"), ()=>this.getMonth(-1)],
        ];

        return (
            <>
              <Dropdown as={InputGroup} className="datetime-selector">
                <Button variant="default-outline">
                  { formatted_ts ? <VeloTimestamp iso={formatted_ts}/> : T("Select Time")}
                </Button>

                {!this.state.showEdit ?
                 <Button
                   onClick={()=>{
                       this.setState({showEdit: true});
                   }}
                   variant="default">
                   <FontAwesomeIcon icon="edit"/>
                 </Button>
                 :
                 <>
                   <Button variant="default"
                           type="submit"
                           disabled={this.state.invalid}
                           onClick={()=>{
                               this.setTime(this.state.value);
                               this.setState({showEdit: false});
                           }}>
                     <FontAwesomeIcon icon="save"/>
                   </Button>
                   <ToolTip tooltip={T("Select time and date in RFC3339 format")}>
                     <Form.Control as="input" rows={1}
                                   spellCheck="false"
                                   className={ this.state.invalid && 'invalid' }
                                   value={this.state.value}

                                   onChange={e=>{
                                       // Check if the time is invalid
                                       // - as the user edits the time
                                       // it may become invalid at any
                                       // time.
                                       let ts = ToStandardTime(e.target.value);
                                       this.setState({
                                           value: e.target.value,
                                           invalid: !_.isDate(ts),
                                       });
                                   }}
                     />
                   </ToolTip>
                 </>
                }

                <Dropdown.Toggle split variant="success">
                  <span className="icon-small">
                    <FontAwesomeIcon icon="sliders"/>
                  </span>
                </Dropdown.Toggle>
                <Dropdown.Menu>
                  <Dropdown.Item className="calendar-selector">
                    <Calendar value={this.props.value}
                              onChange={this.props.onChange}/>
                  </Dropdown.Item>

                  { _.map(presets, (x, idx)=>{
                      let new_ts = x[1]();
                      return <Dropdown.Item
                               key={idx}
                               onClick={e=>this.setTime(
                                   FormatRFC3339(new_ts, timezone))}
                             >
                               {x[0]()}
                             </Dropdown.Item>;
                  })}
                  <Dropdown.Divider />

                </Dropdown.Menu>
              </Dropdown>
              </>
        );
    }
}
